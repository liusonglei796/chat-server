package group

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	"kama_chat_server/internal/common/dto/request/group"
	grouprsp "kama_chat_server/internal/common/dto/respond/group"
	"kama_chat_server/internal/common/grpc_client"
	cacheutil "kama_chat_server/internal/common/infrastructure/cache"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/group/group_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// groupUoW 群组服务的 UoW 接口：事务/事件能力 + 仅本服务拥有的 Group/GroupMember Store
type groupUoW interface {
	store.TxExecutor
	RecordEvent(ctx context.Context, eventType string, payload []byte) error
	GroupStore() store.GroupStore
	GroupMemberStore() store.GroupMemberStore
}

// GroupService 群组业务逻辑实现
// 通过构造函数注入 Store 和 Cache 依赖
type GroupService struct {
	uow         groupUoW
	cache       store.AsyncCacheService
	cacheHelper *cacheutil.Helper // 缓存辅助工具（带 singleflight）
	guard       idempotent.Guard
}

// NewGroupService 构造函数，注入所有依赖
func NewGroupService(uow groupUoW, cacheService store.AsyncCacheService, guard ...idempotent.Guard) *GroupService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	return &GroupService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
		guard:       g,
	}
}

// CreateGroup 创建群聊 (ownerId 从 JWT 获取)
// 包含第三代状态机幂等保护：支持短租约防连击与超时重试直接回显 groupId
func (g *GroupService) CreateGroup(ctx context.Context, ownerId string, groupReq group.CreateGroupRequest) (string, error) {
	// Step 1.1: 构造核心建群业务事务闭包
	// 来源: 入参 ownerId (JWT 用户标识) 与 groupReq 提交的群资料
	// 用于: Step 1.2 注入幂等守卫执行
	action := func(c context.Context) (string, error) {
		newGroup := model.GroupInfo{
			Uuid:      fmt.Sprintf("G%s", snowflake.GenerateIDString()),
			Name:      groupReq.Name,
			Notice:    groupReq.Notice,
			OwnerId:   ownerId, // 使用 JWT 中的用户 ID
			MemberCnt: 1,
			AddMode:   groupReq.AddMode,
			Avatar:    groupReq.Avatar,
			Status:    group_status.NORMAL,
		}

		err := store.WithTx(g.uow, func(tx groupUoW) error {
			// 同群主同名正常群防重复创建
			existing, err := tx.GroupStore().FindByOwnerIdAndName(c, ownerId, newGroup.Name)
			if err == nil && existing != nil {
				return errorx.New(errorx.CodeInvalidParam, "您已拥有同名的正常群聊，请勿重复创建")
			}

			// 持久化群组信息
			if err := tx.GroupStore().CreateGroup(c, &newGroup); err != nil {
				if dberr.IsDuplicateKey(err) {
					return errorx.New(errorx.CodeInvalidParam, "您已拥有同名的正常群聊，请勿重复创建")
				}
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			// 创建群成员（群主 Role=3）
			member := model.GroupMember{
				GroupUuid: newGroup.Uuid,
				UserUuid:  ownerId,
				Role:      3,
			}
			if err := tx.GroupMemberStore().CreateGroupMember(c, &member); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			// 写入 outbox 表 (group_created 事件)，由 Canal CDC 捕获并推至 Kafka 异步解耦
			payload, _ := json.Marshal(event.GroupCreatedEvent{
				GroupId:     newGroup.Uuid,
				OwnerId:     ownerId,
				GroupName:   newGroup.Name,
				GroupAvatar: newGroup.Avatar,
			})
			return tx.RecordEvent(c, event.EventGroupCreated, payload)
		})

		if err != nil {
			var codeErr *errorx.CodeError
			if errors.As(err, &codeErr) {
				return "", err
			}
			zap.L().Error("service error", zap.Error(err))
			return "", errorx.ErrServerBusy
		}
		return newGroup.Uuid, nil
	}

	// Step 1.2: 判定是否启用了客户端防重凭证
	// 来源: groupReq.ClientToken
	// 用于: 若携带防重凭证则通过 Guard 保护，否则直接执行业务
	if groupReq.ClientToken != "" && g.guard != nil {
		key := fmt.Sprintf("grp:create:%s:%s", ownerId, groupReq.ClientToken)
		// 5秒短租约防并发连击，24小时成功持久化防重试回放
		groupID, isReplay, err := g.guard.Execute(ctx, key, 5*time.Second, 24*time.Hour, action)
		if isReplay {
			zap.L().Info("命中建群幂等回放，直接返回已有群号", zap.String("groupId", groupID), zap.String("ownerId", ownerId))
		}
		return groupID, err
	}

	return action(ctx)
}

// LoadMyGroup 获取我创建的群聊（分页）
func (g *GroupService) LoadMyGroup(ctx context.Context, userId string, page, pageSize int) ([]grouprsp.MyGroupListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 数据库分页查询我创建的群组
	groups, total, err := g.uow.GroupStore().FindByOwnerIdPaged(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find my groups by owner id error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	// 构建响应
	myGroups := make([]grouprsp.MyGroupListRespond, 0, len(groups))
	for _, grp := range groups {
		myGroups = append(myGroups, grouprsp.MyGroupListRespond{
			GroupId:   grp.Uuid,
			GroupName: grp.Name,
			Avatar:    grp.Avatar,
		})
	}

	return myGroups, total, nil
}

// GetGroupListByMember 通过 GroupMember 表分页获取用户加入的所有群组
func (g *GroupService) GetGroupListByMember(ctx context.Context, userId string, page, pageSize int) ([]grouprsp.MyGroupListRespond, int64, error) {
	// 参数校验
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 通过 GroupMember 表分页查询用户加入的群组UUID
	groupUuids, total, err := g.uow.GroupMemberStore().FindGroupUuidsByUserPaged(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find group uuids by user paged error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	if len(groupUuids) == 0 {
		return []grouprsp.MyGroupListRespond{}, total, nil
	}

	// 批量获取群组信息
	groups, err := g.uow.GroupStore().FindByUuids(ctx, groupUuids)
	if err != nil {
		zap.L().Error("Batch find groups error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	groupListRsp := make([]grouprsp.MyGroupListRespond, 0, len(groups))
	for _, grp := range groups {
		groupListRsp = append(groupListRsp, grouprsp.MyGroupListRespond{
			GroupId:   grp.Uuid,
			GroupName: grp.Name,
			Avatar:    grp.Avatar,
		})
	}

	return groupListRsp, total, nil
}

// CheckGroupAddMode 检查群聊加群方式
// 使用 Cache-Aside 模式 + singleflight 防止缓存击穿
func (g *GroupService) CheckGroupAddMode(ctx context.Context, groupId string) (int8, error) {
	cacheKey := constants.CacheKeyGroupInfo + groupId
	var groupInfo grouprsp.GetGroupInfoRespond

	err := g.cacheHelper.GetOrLoad(
		ctx,
		cacheKey,
		func(loaderCtx context.Context) (interface{}, error) {
			group, err := g.uow.GroupStore().FindByUuid(loaderCtx, groupId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeNotFound, "群组不存在")
				}
				return nil, err
			}
			return grouprsp.GetGroupInfoRespond{
				Uuid:      group.Uuid,
				Name:      group.Name,
				Notice:    group.Notice,
				MemberCnt: group.MemberCnt,
				OwnerId:   group.OwnerId,
				AddMode:   group.AddMode,
				Status:    group.Status,
				Avatar:    group.Avatar,
				IsDeleted: group.DeletedAt.Valid,
				Version:   group.Version,
			}, nil
		},
		cacheutil.RandomizedTTL(24*time.Hour), // 数据 TTL (带抖动防雪崩)
		5*time.Minute,                         // 空值 TTL (防穿透)
		&groupInfo,
	)
	if err != nil {
		zap.L().Error("Get group info error", zap.String("groupId", groupId), zap.Error(err))
		return -1, err
	}

	return groupInfo.AddMode, nil
}

// LeaveGroup 退群
func (g *GroupService) LeaveGroup(ctx context.Context, userId string, groupId string) error {
	// 校验是否是群成员
	member, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			// 终态幂等自愈：用户已不是群成员（前序退群重试已生效），静默放行并阻断副作用
			return nil
		}
		zap.L().Error("Check group membership error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 群主不能直接退群，必须先转让群主或解散群聊
	if member.Role == 3 {
		return errorx.New(errorx.CodeInvalidParam, "群主不能退群，请先转让群主或解散群聊")
	}

	err = store.WithTx(g.uow, func(tx groupUoW) error {
		deletedCount, err := tx.GroupMemberStore().DeleteByUserUuids(ctx, groupId, []string{userId})
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if deletedCount > 0 {
			if err := tx.GroupStore().DecrementMemberCountBy(ctx, groupId, int(deletedCount)); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			// 写入 outbox 表 (group_member_left 事件)，由 Canal CDC 捕获并推至 Kafka 异步解耦
			payload, _ := json.Marshal(event.GroupMemberLeftEvent{
				GroupId: groupId,
				UserId:  userId,
			})
			return tx.RecordEvent(ctx, event.EventGroupMemberLeft, payload)
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 先同步清理核心群信息与群成员缓存，确保即刻生效防止脏读
	_ = g.cacheHelper.InvalidateWithNull(ctx, constants.CacheKeyGroupInfo+groupId)
	_ = g.cache.Delete(ctx, constants.CacheKeyGroupMembers+groupId)

	g.cache.SubmitTask(func() {
		if err := g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+groupId); err != nil {
			zap.L().Error("清理群信息缓存失败", zap.Error(err))
		}
		if err := g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+groupId); err != nil {
			zap.L().Error("清理群成员缓存失败", zap.Error(err))
		}
	})
	return nil
}

// DismissGroup 解散群聊 (operatorId 必须是群主)
func (g *GroupService) DismissGroup(ctx context.Context, operatorId, groupId string) error {
	// 权限校验: 必须是群主 (Role=3) 才能解散群
	group, err := g.uow.GroupStore().FindByUuid(ctx, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			// 终态幂等自愈：群组已被解散或不存在，静默返回成功
			return nil
		}
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if group.OwnerId != operatorId {
		return errorx.New(errorx.CodeForbidden, "只有群主才能解散群聊")
	}

	err = store.WithTx(g.uow, func(tx groupUoW) error {
		// 1. 删除所有群成员
		if err := tx.GroupMemberStore().DeleteByGroupUuid(ctx, groupId); err != nil {
			zap.L().Error("Delete members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 2. 软删除群组
		if err := tx.GroupStore().SoftDeleteByUuids(ctx, []string{groupId}); err != nil {
			zap.L().Error("Soft delete group error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 3. 写入 outbox 表 (group_dismissed 事件)，由 Canal CDC 捕获并推至 Kafka 异步解耦
		payload, _ := json.Marshal(event.GroupDismissedEvent{
			GroupId: groupId,
		})
		return tx.RecordEvent(ctx, event.EventGroupDismissed, payload)
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 精确清理 Redis 缓存
	_ = g.cacheHelper.InvalidateWithNull(ctx, constants.CacheKeyGroupInfo+groupId)
	_ = g.cache.Delete(ctx, constants.CacheKeyGroupMembers+groupId)

	g.cache.SubmitTask(func() {
		// 清理群公共信息（含空值标记）
		if err := g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+groupId); err != nil {
			zap.L().Error("清理群信息缓存失败", zap.Error(err))
		}
		if err := g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+groupId); err != nil {
			zap.L().Error("清理群成员缓存失败", zap.Error(err))
		}
	})

	return nil
}

// UpdateGroupInfo 更新群组信息 (operatorId 必须是群主或管理员)
func (g *GroupService) UpdateGroupInfo(ctx context.Context, operatorId string, req group.UpdateGroupInfoRequest) error {
	// Step 1.1: 权限校验
	// 来源: req.Uuid 与 operatorId
	// 用于: Step 1.2 确认操作人具备群主或管理员 (Role >= 2) 权限
	member, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, req.Uuid, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if member.Role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有修改权限")
	}

	// Step 1.2: 版本号入参校验
	// 来源: req.Version
	// 用于: Step 1.4 乐观锁 CAS 防并发覆盖
	if req.Version == nil {
		return errorx.New(errorx.CodeInvalidParam, "缺少版本号")
	}

	// Step 1.3: 收集非空更新字段
	// 来源: req 中的各项属性
	// 用于: Step 1.4 仅更新提交的字段，防止未传字段被置空
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.AddMode != nil {
		updates["add_mode"] = *req.AddMode
	}
	if req.Notice != nil {
		updates["notice"] = *req.Notice
	}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}

	if len(updates) == 0 {
		return nil
	}

	// Step 1.4: 开启事务执行带版本号的乐观锁 CAS 更新
	// 来源: req.Uuid, *req.Version 及 updates 字段集合
	// 用于: 保证并发修改不丢失更新；当版本冲突时执行字段内容比对自愈
	if err := store.WithTx(g.uow, func(tx groupUoW) error {
		return tx.GroupStore().UpdateWithVersion(ctx, req.Uuid, *req.Version, updates)
	}); err != nil {
		if errorx.IsCode(err, errorx.CodeConflict) {
			// Step 1.4.1: 乐观锁冲突时的幂等自愈比对
			// 来源: 冲突时重新查出的群组最新记录
			// 用于: 判断当前数据库中字段是否已完全满足本次修改；若已达成则作为网络重试自愈成功
			currentGroup, queryErr := g.uow.GroupStore().FindByUuid(ctx, req.Uuid)
			if queryErr == nil && currentGroup != nil {
				matched := true
				if req.Name != nil && currentGroup.Name != *req.Name {
					matched = false
				}
				if req.AddMode != nil && currentGroup.AddMode != *req.AddMode {
					matched = false
				}
				if req.Notice != nil && currentGroup.Notice != *req.Notice {
					matched = false
				}
				if req.Avatar != nil && currentGroup.Avatar != *req.Avatar {
					matched = false
				}
				if matched {
					zap.L().Info("UpdateGroupInfo 命中乐观锁版本变更但字段内容完全一致，幂等自愈返回成功",
						zap.String("groupId", req.Uuid),
						zap.Int32("expectedVersion", *req.Version),
						zap.Int32("currentVersion", currentGroup.Version),
					)
					return nil
				}
			}
			return err
		}
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 1.5: 异步清理群信息缓存
	// 来源: req.Uuid
	// 用于: 失效旧缓存，驱动下次读取时加载最新资料
	groupId := req.Uuid
	g.cache.SubmitTask(func() {
		if err := g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+groupId); err != nil {
			zap.L().Error("清理群信息缓存失败", zap.Error(err))
		}
	})

	return nil
}

// GetGroupMemberList 获取群聊成员列表（分页）(userId 必须是群成员)
func (g *GroupService) GetGroupMemberList(ctx context.Context, userId, groupId string, page, pageSize int) ([]grouprsp.GetGroupMemberListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 权限校验: 必须是群成员
	_, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, 0, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	// 查询全部群成员（仅 group_member 表，不 JOIN user 表）
	members, err := g.uow.GroupMemberStore().FindByGroupUuid(ctx, groupId)
	if err != nil {
		zap.L().Error("Find group members error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	total := int64(len(members))

	// 批量获取成员公开信息（昵称/头像），避免直读 user 表
	userIds := make([]string, 0, len(members))
	for _, m := range members {
		userIds = append(userIds, m.UserUuid)
	}
	userList, err := grpc_client.BatchGetPublicUserInfo(ctx, userIds)
	if err != nil {
		zap.L().Error("batch get group members info via grpc error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}
	userMap := make(map[string]*userpb.PublicUserInfo, len(userList))
	for _, u := range userList {
		userMap[u.Uuid] = u
	}

	// 按分页参数切片返回
	start := (page - 1) * pageSize
	if start >= len(members) {
		return []grouprsp.GetGroupMemberListRespond{}, total, nil
	}
	end := start + pageSize
	if end > len(members) {
		end = len(members)
	}

	rspList := make([]grouprsp.GetGroupMemberListRespond, 0, end-start)
	for _, m := range members[start:end] {
		u, ok := userMap[m.UserUuid]
		if !ok {
			continue
		}
		rspList = append(rspList, grouprsp.GetGroupMemberListRespond{
			UserId:   u.Uuid,
			Nickname: u.Nickname,
			Avatar:   u.Avatar,
		})
	}

	return rspList, total, nil
}

// RemoveGroupMembers 移除群聊成员 (高危操作：需群主或管理员权限)
//
// 业务与架构流程：
//  1. RBAC 鉴权校验：operatorId 必须是群内成员且具备管理权限 (Role >= 2)，防御非法调用
//  2. 保护性约束：遍历待踢名单，严格禁止踢出群主 (Owner)
//  3. 本地事务原子化 (Transactional Outbox 模式)：
//     - 在同一 DB 事务内执行：物理/逻辑删除成员、原子递减群成员总数
//     - 在事务内写入 EventGroupMemberRemoved 领域事件到 outbox 表，杜绝“先改DB再发MQ失败”造成的分布式双写不一致
//  4. 缓存强淘汰与防越权 (Defense-in-Depth)：
//     - 事务提交后，立即同步淘汰 Redis 群信息与群成员列表缓存 (耗时 < 1ms)，消除并发发信时的权限脏读时间窗
//     - 异步协程池兜底清理，确保服务高可用与容错
func (g *GroupService) RemoveGroupMembers(ctx context.Context, operatorId string, req group.RemoveGroupMembersRequest) error {
	// 参数边界检查：空列表直接返回
	if len(req.UuidList) == 0 {
		return nil
	}

	// 1. RBAC 权限校验: 检查操作者是否在群内且角色是否为群主或管理员 (Role >= 2)
	member, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, req.GroupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if member.Role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有移除成员的权限")
	}

	// 2. 核心保护约束: 获取群元信息，禁止任何操作者移除群主
	group, err := g.uow.GroupStore().FindByUuid(ctx, req.GroupId)
	if err != nil {
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	for _, uuid := range req.UuidList {
		if group.OwnerId == uuid {
			return errorx.New(errorx.CodeInvalidParam, "不能移除群主")
		}
	}

	// 3. 本地事务执行原子操作 (Transactional Outbox 模式)
	err = store.WithTx(g.uow, func(tx groupUoW) error {
		// 3.1 批量删除群成员关系记录，获取实际删除行数
		deletedCount, err := tx.GroupMemberStore().DeleteByUserUuids(ctx, req.GroupId, req.UuidList)
		if err != nil {
			zap.L().Error("Delete group members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 3.2 只有实际成功删除了成员，才扣减群总人数并派发事件（严格防重复扣减）
		if deletedCount > 0 {
			if err := tx.GroupStore().DecrementMemberCountBy(ctx, req.GroupId, int(deletedCount)); err != nil {
				zap.L().Error("Decrement member count error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			// 3.3 写入 outbox 表 (group_member_removed 事件)，由 Canal CDC 捕获并推至 Kafka 异步解耦
			payload, _ := json.Marshal(event.GroupMemberRemovedEvent{
				GroupId:     req.GroupId,
				OperatorId:  operatorId,
				MemberUuids: req.UuidList,
			})
			return tx.RecordEvent(ctx, event.EventGroupMemberRemoved, payload)
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 4. 多级缓存一致性与越权防御：
	// 先在主流程同步执行 Redis 淘汰，确保下一毫秒的群聊分发查不到已踢人员，杜绝机密消息泄露
	_ = g.cacheHelper.InvalidateWithNull(ctx, constants.CacheKeyGroupInfo+req.GroupId)
	_ = g.cache.Delete(ctx, constants.CacheKeyGroupMembers+req.GroupId)

	// 异步协程池兜底再次清理，防止网络抖动并兼顾系统高可用
	g.cache.SubmitTask(func() {
		// 清理群本身的缓存（含防击穿空值标记）
		if err := g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+req.GroupId); err != nil {
			zap.L().Error("清理群信息缓存失败", zap.Error(err))
		}
		if err := g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+req.GroupId); err != nil {
			zap.L().Error("清理群成员缓存失败", zap.Error(err))
		}
	})

	return nil
}

// GetGroupDetail 获取群聊详情 (userId 必须是群成员)
func (g *GroupService) GetGroupDetail(ctx context.Context, userId, groupId string) (grouprsp.PublicGroupInfoRespond, error) {
	if len(groupId) == 0 {
		return grouprsp.PublicGroupInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "群聊ID不能为空")
	}

	// 权限校验: 必须是群成员
	_, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return grouprsp.PublicGroupInfoRespond{}, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Check group membership error", zap.Error(err))
		return grouprsp.PublicGroupInfoRespond{}, errorx.ErrServerBusy
	}

	cacheKey := constants.CacheKeyGroupInfo + groupId
	var groupInfo grouprsp.GetGroupInfoRespond

	err = g.cacheHelper.GetOrLoad(
		ctx,
		cacheKey,
		func(loaderCtx context.Context) (interface{}, error) {
			grp, err := g.uow.GroupStore().FindByUuid(loaderCtx, groupId)
			if err != nil {
				if errorx.IsNotFound(err) {
					return nil, errorx.New(errorx.CodeNotFound, "该群聊不存在")
				}
				return nil, err
			}
			if grp.Status == group_status.DISABLE {
				return nil, errorx.New(errorx.CodeInvalidParam, "该群聊处于禁用状态")
			}
			return grouprsp.GetGroupInfoRespond{
				Uuid:      grp.Uuid,
				Name:      grp.Name,
				Notice:    grp.Notice,
				Avatar:    grp.Avatar,
				MemberCnt: grp.MemberCnt,
				OwnerId:   grp.OwnerId,
				AddMode:   grp.AddMode,
				Status:    grp.Status,
				IsDeleted: grp.DeletedAt.Valid,
				Version:   grp.Version,
			}, nil
		},
		cacheutil.RandomizedTTL(time.Hour),
		5*time.Minute,
		&groupInfo,
	)
	if err != nil {
		return grouprsp.PublicGroupInfoRespond{}, err
	}

	return grouprsp.PublicGroupInfoRespond{
		GroupId:     groupInfo.Uuid,
		GroupName:   groupInfo.Name,
		GroupAvatar: groupInfo.Avatar,
		GroupNotice: groupInfo.Notice,
		MemberCnt:   groupInfo.MemberCnt,
		OwnerId:     groupInfo.OwnerId,
		AddMode:     groupInfo.AddMode,
	}, nil
}

// MuteMember 禁言/取消禁言群成员
func (g *GroupService) MuteMember(ctx context.Context, operatorId string, req group.MuteMemberRequest) error {
	// 1. 权限校验: 必须是群主或管理员 (Role >= 2)
	operator, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, req.GroupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if operator.Role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有禁言成员的权限")
	}

	// 2. 不能禁言群主
	grp, err := g.uow.GroupStore().FindByUuid(ctx, req.GroupId)
	if err != nil {
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if grp.OwnerId == req.UserId {
		return errorx.New(errorx.CodeInvalidParam, "不能禁言群主")
	}

	// 3. 校验被操作者是群成员
	_, err = g.uow.GroupMemberStore().FindByGroupAndUser(ctx, req.GroupId, req.UserId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "该用户不是群成员")
		}
		zap.L().Error("Find target member error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 4. 设置禁言
	var muteUntil *time.Time
	if req.Duration > 0 {
		t := time.Now().Add(time.Duration(req.Duration) * time.Minute)
		muteUntil = &t
	}

	if err := g.uow.GroupMemberStore().UpdateMuteUntil(ctx, req.GroupId, req.UserId, muteUntil); err != nil {
		zap.L().Error("Update mute until error",
			zap.String("group_id", req.GroupId),
			zap.String("user_id", req.UserId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	// 5. 清理群成员缓存
	g.cache.SubmitTask(func() {
		if err := g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+req.GroupId); err != nil {
			zap.L().Error("清理群成员缓存失败", zap.Error(err))
		}
	})

	return nil
}

// IsGroupMember 检查用户是否为群成员
func (g *GroupService) IsGroupMember(ctx context.Context, groupId, userId string) (bool, error) {
	_, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return false, nil
		}
		zap.L().Error("query group member error", zap.Error(err))
		return false, errorx.ErrServerBusy
	}
	return true, nil
}

// GetGroupMemberRole 返回用户在某群的角色（1普通成员 2管理员 3群主）
// 供 apply 服务跨库校验管理员权限，避免跨库直连 group_member 表
func (g *GroupService) GetGroupMemberRole(ctx context.Context, groupId, userId string) (int8, error) {
	member, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return 0, errorx.New(errorx.CodeNotFound, "该用户不是群成员")
		}
		zap.L().Error("find group member error", zap.Error(err))
		return 0, errorx.ErrServerBusy
	}
	return member.Role, nil
}

// ListGroupMemberIds 返回群全部成员ID（直接读 group_member 表，不 JOIN user 表）
func (g *GroupService) ListGroupMemberIds(ctx context.Context, groupId string) ([]string, error) {
	members, err := g.uow.GroupMemberStore().FindByGroupUuid(ctx, groupId)
	if err != nil {
		zap.L().Error("query group members error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserUuid)
	}
	return ids, nil
}

// AddGroupMember 跨服务向群组中添加成员（供 apply 审批通过后直调）
func (g *GroupService) AddGroupMember(ctx context.Context, groupId, userId string, role int8) error {
	err := store.WithTx(g.uow, func(tx groupUoW) error {
		newMember := model.GroupMember{
			GroupUuid: groupId,
			UserUuid:  userId,
			Role:      role,
		}
		if err := tx.GroupMemberStore().CreateGroupMember(ctx, &newMember); err != nil {
			zap.L().Error("CreateGroupMember error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if err := tx.GroupStore().IncrementMemberCount(ctx, groupId); err != nil {
			zap.L().Error("IncrementMemberCount error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 同步淘汰群成员与群信息缓存
	_ = g.cacheHelper.InvalidateWithNull(ctx, constants.CacheKeyGroupInfo+groupId)
	_ = g.cache.Delete(ctx, constants.CacheKeyGroupMembers+groupId)

	return nil
}

