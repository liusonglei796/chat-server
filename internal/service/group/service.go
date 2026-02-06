package group

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request/group"
	grouprsp "kama_chat_server/internal/dto/respond/group"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/contact/contact_type_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/errorx"
	cacheutil "kama_chat_server/pkg/util/cache"
)

// groupInfoService 群组业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖
type groupInfoService struct {
	repos       *mysql.Repositories
	cache       myredis.AsyncCacheService
	cacheHelper *cacheutil.Helper // 缓存辅助工具（带 singleflight）
}

// NewGroupService 构造函数，注入所有依赖
func NewGroupService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *groupInfoService {
	return &groupInfoService{
		repos:       repos,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
	}
}

// CreateGroup 创建群聊 (ownerId 从 JWT 获取)
func (g *groupInfoService) CreateGroup(ownerId string, groupReq group.CreateGroupRequest) error {
	group := model.GroupInfo{
		Uuid:      fmt.Sprintf("G%s", snowflake.GenerateIDString()),
		Name:      groupReq.Name,
		Notice:    groupReq.Notice,
		OwnerId:   ownerId, // 使用 JWT 中的用户 ID
		MemberCnt: 1,
		AddMode:   groupReq.AddMode,
		Avatar:    groupReq.Avatar,
		Status:    group_status_enum.NORMAL,
	}

	err := g.repos.Transaction(func(txRepos *mysql.Repositories) error {
		if err := txRepos.Group.CreateGroup(&group); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 创建群成员
		member := model.GroupMember{
			GroupUuid: group.Uuid,
			UserUuid:  ownerId,
			Role:      3,
		}
		if err := txRepos.GroupMember.CreateGroupMember(&member); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 添加联系
		contact := model.Contact{
			UserId:      ownerId,
			ContactId:   group.Uuid,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&contact); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 创建会话
		session := model.Session{
			Uuid:        fmt.Sprintf("S%s", snowflake.GenerateIDString()),
			SendId:      ownerId,
			ReceiveId:   group.Uuid,
			ReceiveName: group.Name,
			Avatar:      group.Avatar,
		}
		if err := txRepos.Session.CreateSession(&session); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	g.cache.SubmitTask(func() {
		// 删除联系关系缓存
		if err := g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+ownerId+"*"); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		// 删除群组会话列表缓存
		if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+ownerId+"*"); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
	})

	return nil
}

// LoadMyGroup 获取我创建的群聊（分页）
func (g *groupInfoService) LoadMyGroup(userId string, page, pageSize int) ([]grouprsp.MyGroupListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 数据库分页查询我创建的群组
	groups, total, err := g.repos.Group.FindByOwnerIdPaged(userId, page, pageSize)
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

// GetJoinedGroups 获取我加入的群组（包含自己创建的）
// 注意：如果业务层需要区分"我加入的" vs "我创建的"，可以在 Handler 层过滤，
// 或者复用 getGroupsByUserId 自行处理。
// 这里为了满足 Contact 模块的需求，通常是指"所有群组"或"非我创建的群组"，
// 之前的 Contact.GetJoinedGroupsExcludedOwn 是排除自己创建的。
// 我们这里提供 GetJoinedGroupsExcludedOwn 对应的逻辑：
func (g *groupInfoService) GetJoinedGroups(userId string) ([]grouprsp.MyGroupListRespond, error) {
	// 1. 获取所有关联群组
	allGroups, err := g.getGroupsByUserId(userId)
	if err != nil {
		return nil, err
	}

	// 2. 过滤出我加入的群组 (排除自己创建的)
	groupListRsp := make([]grouprsp.MyGroupListRespond, 0)
	for _, grp := range allGroups {
		if grp.OwnerId != userId {
			groupListRsp = append(groupListRsp, grouprsp.MyGroupListRespond{
				GroupId:   grp.Uuid,
				GroupName: grp.Name,
				Avatar:    grp.Avatar,
			})
		}
	}
	return groupListRsp, nil
}

// getGroupsByUserId 获取用户所有关联的群组信息（创建的 + 加入的）
// 封装了 Cache-Aside 逻辑
func (g *groupInfoService) getGroupsByUserId(userId string) ([]model.GroupInfo, error) {
	cacheKey := "contact_relation:group:" + userId

	// 1. 尝试从缓存获取所有已加入的群组ID
	groupUuids, err := g.cache.GetSetMembers(context.Background(), cacheKey)
	if err != nil || len(groupUuids) == 0 {
		// 2. 缓存未击中：从数据库获取
		contactList, _, dbErr := g.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP, 1, 1000)
		if dbErr != nil {
			zap.L().Error("Find my groups contact error", zap.Error(dbErr))
			return nil, errorx.ErrServerBusy
		}

		groupUuids = make([]string, 0, len(contactList))
		for _, c := range contactList {
			if len(c.ContactId) > 0 && c.ContactId[0] == 'G' {
				groupUuids = append(groupUuids, c.ContactId)
			}
		}

		// 回写到缓存
		if len(groupUuids) > 0 {
			args := make([]interface{}, len(groupUuids))
			for i, v := range groupUuids {
				args[i] = v
			}
			_ = g.cache.AddToSet(context.Background(), cacheKey, args...)
		}
	}

	if len(groupUuids) == 0 {
		return []model.GroupInfo{}, nil
	}

	// 3. 批量获取群组信息
	var result []model.GroupInfo
	missingIds := make([]string, 0)

	for _, groupId := range groupUuids {
		infoKey := "group_info_" + groupId
		val, err := g.cache.Get(context.Background(), infoKey)
		if err == nil && val != "" {
			// 注意：缓存里存的是 grouprsp.GetGroupInfoRespond 结构，不是 model.GroupInfo
			var dto grouprsp.GetGroupInfoRespond
			if err := json.Unmarshal([]byte(val), &dto); err == nil {
				// DTO -> Model (部分字段)
				result = append(result, model.GroupInfo{
					Uuid:    dto.Uuid,
					Name:    dto.Name,
					Avatar:  dto.Avatar,
					OwnerId: dto.OwnerId,
				})
			} else {
				// 反序列化失败，视为缓存脏数据，需要从数据库获取
				missingIds = append(missingIds, groupId)
			}
		} else {
			// 缓存未命中，需要从数据库获取
			missingIds = append(missingIds, groupId)
		}
	}

	// 4. 处理缺失的详情
	if len(missingIds) > 0 {
		groups, err := g.repos.Group.FindByUuids(missingIds)
		if err != nil {
			zap.L().Error("Batch find groups error", zap.Error(err))
			return nil, errorx.ErrServerBusy
		}

		for _, grp := range groups {
			result = append(result, grp)

			// 异步回写缓存
			cacheGroup := grp
			g.cache.SubmitTask(func() {
				info := grouprsp.GetGroupInfoRespond{
					Uuid:      cacheGroup.Uuid,
					Name:      cacheGroup.Name,
					Notice:    cacheGroup.Notice,
					MemberCnt: cacheGroup.MemberCnt,
					OwnerId:   cacheGroup.OwnerId,
					AddMode:   cacheGroup.AddMode,
					Status:    cacheGroup.Status,
					Avatar:    cacheGroup.Avatar,
					IsDeleted: cacheGroup.DeletedAt.Valid,
				}
				if data, err := json.Marshal(info); err == nil {
					_ = g.cache.Set(context.Background(), "group_info_"+cacheGroup.Uuid, string(data), time.Hour*24)
				}
			})
		}
	}

	return result, nil
}

// CheckGroupAddMode 检查群聊加群方式
// 使用 Cache-Aside 模式 + singleflight 防止缓存击穿
func (g *groupInfoService) CheckGroupAddMode(groupId string) (int8, error) {
	cacheKey := "group_info_" + groupId
	var groupInfo grouprsp.GetGroupInfoRespond

	err := g.cacheHelper.GetOrLoad(
		context.Background(),
		cacheKey,
		func() (interface{}, error) {
			group, err := g.repos.Group.FindByUuid(groupId)
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
func (g *groupInfoService) LeaveGroup(userId string, groupId string) error {
	// 校验是否是群成员
	_, err := g.repos.GroupMember.FindByGroupAndUser(groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Check group membership error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	err = g.repos.Transaction(func(txRepos *mysql.Repositories) error {
		if err := txRepos.GroupMember.DeleteByUserUuids(groupId, []string{userId}); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if err := txRepos.Group.DecrementMemberCountBy(groupId, 1); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 3. (无需删除会话，保留历史)
		// session, _ := txRepos.Session.FindBySendIdAndReceiveId(userId, groupId)
		// if session != nil {
		// 	if err := txRepos.Session.SoftDeleteByUuids([]string{session.Uuid}); err != nil {
		// 		zap.L().Error("service error", zap.Error(err))
		// 	}
		// }

		if err := txRepos.Contact.SoftDelete(userId, groupId, contact_type_enum.GROUP); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if err := txRepos.Apply.SoftDelete(userId, groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	g.cache.SubmitTask(func() {
		if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+userId+"*"); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		if err := g.cache.RemoveFromSet(context.Background(), "contact_relation:group:"+userId, groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		if err := g.cache.Delete(context.Background(), "group_info_"+groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		if err := g.cache.Delete(context.Background(), "group_memberlist_"+groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
	})
	return nil
}

// DismissGroup 解散群聊 (operatorId 必须是群主)
func (g *groupInfoService) DismissGroup(operatorId, groupId string) error {
	// 权限校验: 必须是群主 (Role=3) 才能解散群
	group, err := g.repos.Group.FindByUuid(groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "群组不存在")
		}
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if group.OwnerId != operatorId {
		return errorx.New(errorx.CodeForbidden, "只有群主才能解散群聊")
	}

	var memberIds []string

	err = g.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 1. 获取涉及的成员ID
		members, err := txRepos.GroupMember.FindByGroupUuid(groupId)
		if err != nil {
			zap.L().Error("Find members by group id error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		for _, m := range members {
			memberIds = append(memberIds, m.UserUuid)
		}

		// 2. 删除所有群成员
		if err := txRepos.GroupMember.DeleteByGroupUuid(groupId); err != nil {
			zap.L().Error("Delete members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 3. 软删除群组
		if err := txRepos.Group.SoftDeleteByUuids([]string{groupId}); err != nil {
			zap.L().Error("Soft delete group error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 4. 软删除所有相关的会话
		if err := txRepos.Session.SoftDeleteByUsers([]string{groupId}); err != nil {
			zap.L().Error("Soft delete sessions error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 5. 批量软删除涉及该群的联系关系
		if err := txRepos.Contact.SoftDeleteByUsers([]string{groupId}); err != nil {
			zap.L().Error("Soft delete contacts error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 6. 批量软删除涉及该群的申请记录
		if err := txRepos.Apply.SoftDeleteByUsers([]string{groupId}); err != nil {
			zap.L().Error("Soft delete applies error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 7. 精确清理 Redis 缓存 (事务外)
	g.cache.SubmitTask(func() {
		// 清理群主的缓存
		if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+operatorId+"*"); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		// 清理所有群成员的缓存
		for _, memberId := range memberIds {
			if err := g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memberId+"*"); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
			if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memberId+"*"); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
		}

		// 清理群公共信息
		if err := g.cache.Delete(context.Background(), "group_info_"+groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		if err := g.cache.Delete(context.Background(), "group_memberlist_"+groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
	})

	return nil
}

// GetPublicGroupInfo 获取群组公开信息（非群成员也可查看）
// 使用 Cache-Aside 模式 + singleflight 防止缓存击穿
func (g *groupInfoService) GetPublicGroupInfo(groupId string) (*grouprsp.PublicGroupInfoRespond, error) {
	cacheKey := "group_info_" + groupId
	var fullInfo grouprsp.GetGroupInfoRespond

	err := g.cacheHelper.GetOrLoad(
		context.Background(),
		cacheKey,
		func() (interface{}, error) {
			group, err := g.repos.Group.FindByUuid(groupId)
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
			}, nil
		},
		cacheutil.RandomizedTTL(24*time.Hour), // 数据 TTL (带抖动防雪崩)
		5*time.Minute,                         // 空值 TTL (防穿透)
		&fullInfo,
	)
	if err != nil {
		return nil, err
	}

	// 转换为公开信息（只包含非敏感字段）
	return &grouprsp.PublicGroupInfoRespond{
		Uuid:      fullInfo.Uuid,
		Name:      fullInfo.Name,
		Notice:    fullInfo.Notice,
		Avatar:    fullInfo.Avatar,
		MemberCnt: fullInfo.MemberCnt,
		AddMode:   fullInfo.AddMode,
	}, nil
}

// UpdateGroupInfo 更新群组信息 (operatorId 必须是群主或管理员)
func (g *groupInfoService) UpdateGroupInfo(operatorId string, req group.UpdateGroupInfoRequest) error {
	// 权限校验: 必须是群主或管理员 (Role >= 2)
	member, err := g.repos.GroupMember.FindByGroupAndUser(req.Uuid, operatorId)
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

	group, err := g.repos.Group.FindByUuid(req.Uuid)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 更新字段
	if req.Name != "" {
		group.Name = req.Name
	}
	if req.AddMode != -1 {
		group.AddMode = req.AddMode
	}
	if req.Notice != "" {
		group.Notice = req.Notice
	}
	if req.Avatar != "" {
		group.Avatar = req.Avatar
	}

	if err := g.repos.Group.Update(group); err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 批量更新 Session
	sessionUpdates := map[string]interface{}{
		"receive_name": group.Name,
		"avatar":       group.Avatar,
	}
	if err := g.repos.Session.UpdateByReceiveId(req.Uuid, sessionUpdates); err != nil {
		zap.L().Error("service error", zap.Error(err))
	}

	// 获取群成员列表用于缓存清理
	members, _ := g.repos.GroupMember.FindByGroupUuid(req.Uuid)
	memberIds := make([]string, 0, len(members))
	for _, m := range members {
		memberIds = append(memberIds, m.UserUuid)
	}

	// 异步清理缓存
	groupId := req.Uuid
	g.cache.SubmitTask(func() {
		// 清理群信息缓存
		if err := g.cache.Delete(context.Background(), "group_info_"+groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		// 清理所有群成员的会话列表缓存
		for _, memberId := range memberIds {
			if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memberId+"*"); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
		}
	})

	return nil
}

// GetGroupMemberList 获取群聊成员列表（分页）(userId 必须是群成员)
func (g *groupInfoService) GetGroupMemberList(userId, groupId string, page, pageSize int) ([]grouprsp.GetGroupMemberListRespond, int64, error) {
	// 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 权限校验: 必须是群成员
	_, err := g.repos.GroupMember.FindByGroupAndUser(groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, 0, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	// 分页查询数据库
	members, total, err := g.repos.GroupMember.FindMembersWithUserInfoPaged(groupId, page, pageSize)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	// 构建响应
	rspList := make([]grouprsp.GetGroupMemberListRespond, 0, len(members))
	for _, m := range members {
		rspList = append(rspList, grouprsp.GetGroupMemberListRespond{
			UserId:   m.UserId,
			Nickname: m.Nickname,
			Avatar:   m.Avatar,
		})
	}

	return rspList, total, nil
}

// RemoveGroupMembers 移除群聊成员 (operatorId 必须是群主或管理员)
func (g *groupInfoService) RemoveGroupMembers(operatorId string, req group.RemoveGroupMembersRequest) error {
	if len(req.UuidList) == 0 {
		return nil
	}

	// 1. 权限校验: 必须是群主或管理员 (Role >= 2)
	member, err := g.repos.GroupMember.FindByGroupAndUser(req.GroupId, operatorId)
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

	// 2. 获取群主 ID（不允许移除群主）
	group, err := g.repos.Group.FindByUuid(req.GroupId)
	if err != nil {
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	for _, uuid := range req.UuidList {
		if group.OwnerId == uuid {
			return errorx.New(errorx.CodeInvalidParam, "不能移除群主")
		}
	}

	// 3. 事务执行删除操作
	err = g.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 删除群成员
		if err := txRepos.GroupMember.DeleteByUserUuids(req.GroupId, req.UuidList); err != nil {
			zap.L().Error("Delete group members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 批量减少成员数
		if err := txRepos.Group.DecrementMemberCountBy(req.GroupId, len(req.UuidList)); err != nil {
			zap.L().Error("Decrement member count error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除 Contact 和 Apply
		for _, uuid := range req.UuidList {
			if err := txRepos.Contact.SoftDelete(uuid, req.GroupId, contact_type_enum.GROUP); err != nil {
				zap.L().Error("Delete contact error", zap.Error(err))
			}
			if err := txRepos.Apply.SoftDelete(uuid, req.GroupId); err != nil {
				zap.L().Error("Delete contact apply error", zap.Error(err))
			}
		}

		// 软删除 Session (无需删除，保留历史)
		// if err := txRepos.Session.SoftDeleteByUsers([]string{req.GroupId}); err != nil {
		// 	zap.L().Error("Delete sessions error", zap.Error(err))
		// }

		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 4. 异步精确清理缓存
	g.cache.SubmitTask(func() {
		// 清理被移除成员的缓存
		for _, memId := range req.UuidList {
			if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memId+"*"); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
			if err := g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memId+"*"); err != nil {
				zap.L().Error("service error", zap.Error(err))
			}
		}
		// 清理群本身的缓存
		if err := g.cache.Delete(context.Background(), "group_info_"+req.GroupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
		if err := g.cache.Delete(context.Background(), "group_memberlist_"+req.GroupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
	})

	return nil
}
