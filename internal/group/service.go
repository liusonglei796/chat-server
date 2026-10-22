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
	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/internal/infra/snowflake"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/group/group_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/event"
	"kama_chat_server/pkg/idempotent"
)

// GroupService 群组业务逻辑实现
type GroupService struct {
	uow         UoW
	cache       cacheinfra.AsyncCacheService
	cacheHelper *cacheinfra.Helper
	guard       idempotent.Guard
}

// NewGroupService 构造函数，注入所有依赖
func NewGroupService(uow UoW, cacheService cacheinfra.AsyncCacheService, guard ...idempotent.Guard) *GroupService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	var helper *cacheinfra.Helper
	if cacheService != nil {
		helper = cacheinfra.NewHelper(cacheService)
	}
	return &GroupService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: helper,
		guard:       g,
	}
}

// CreateGroup 创建群聊 (ownerId 从 JWT 获取)
func (g *GroupService) CreateGroup(ctx context.Context, ownerId string, groupReq CreateGroupRequest) (string, error) {
	action := func(c context.Context) (string, error) {
		newGroup := GroupInfo{
			Uuid:      fmt.Sprintf("G%s", snowflake.GenerateIDString()),
			Name:      groupReq.Name,
			Notice:    groupReq.Notice,
			OwnerId:   ownerId,
			MemberCnt: 1,
			AddMode:   groupReq.AddMode,
			Avatar:    groupReq.Avatar,
			Status:    group_status.NORMAL,
		}

		err := WithTx(g.uow, func(tx UoW) error {
			existing, err := tx.GroupStore().FindByOwnerIdAndName(c, ownerId, newGroup.Name)
			if err == nil && existing != nil {
				return errorx.New(errorx.CodeInvalidParam, "您已拥有同名的正常群聊，请勿重复创建")
			}

			if err := tx.GroupStore().CreateGroup(c, &newGroup); err != nil {
				if dberr.IsDuplicateKey(err) {
					return errorx.New(errorx.CodeInvalidParam, "您已拥有同名的正常群聊，请勿重复创建")
				}
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			member := GroupMember{
				GroupUuid: newGroup.Uuid,
				UserUuid:  ownerId,
				Role:      3,
			}
			if err := tx.GroupMemberStore().CreateGroupMember(c, &member); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			payload, _ := json.Marshal(event.GroupCreatedEvent{
				GroupId:     newGroup.Uuid,
				OwnerId:     ownerId,
				GroupName:   newGroup.Name,
				GroupAvatar: newGroup.Avatar,
			})
			return tx.RecordEvent(c, newGroup.Uuid, event.EventGroupCreated, payload)
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

	if groupReq.ClientToken != "" && g.guard != nil {
		key := fmt.Sprintf("grp:create:%s:%s", ownerId, groupReq.ClientToken)
		groupID, isReplay, err := g.guard.Execute(ctx, key, 5*time.Second, 24*time.Hour, action)
		if isReplay {
			zap.L().Info("命中建群幂等回放，直接返回已有群号", zap.String("groupId", groupID), zap.String("ownerId", ownerId))
		}
		return groupID, err
	}

	return action(ctx)
}

// LoadMyGroup 获取我创建的群聊（分页）
func (g *GroupService) LoadMyGroup(ctx context.Context, userId string, page, pageSize int) ([]MyGroupListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	groups, total, err := g.uow.GroupStore().FindByOwnerIdPaged(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find my groups by owner id error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	myGroups := make([]MyGroupListRespond, 0, len(groups))
	for _, grp := range groups {
		myGroups = append(myGroups, MyGroupListRespond{
			GroupId:   grp.Uuid,
			GroupName: grp.Name,
			Avatar:    grp.Avatar,
		})
	}

	return myGroups, total, nil
}

// GetGroupListByMember 通过 GroupMember 表分页获取用户加入的所有群组
func (g *GroupService) GetGroupListByMember(ctx context.Context, userId string, page, pageSize int) ([]MyGroupListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	groupUuids, total, err := g.uow.GroupMemberStore().FindGroupUuidsByUserPaged(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find group uuids by user paged error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	if len(groupUuids) == 0 {
		return []MyGroupListRespond{}, total, nil
	}

	groups, err := g.uow.GroupStore().FindByUuids(ctx, groupUuids)
	if err != nil {
		zap.L().Error("Batch find groups error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	groupListRsp := make([]MyGroupListRespond, 0, len(groups))
	for _, grp := range groups {
		groupListRsp = append(groupListRsp, MyGroupListRespond{
			GroupId:   grp.Uuid,
			GroupName: grp.Name,
			Avatar:    grp.Avatar,
		})
	}

	return groupListRsp, total, nil
}

// CheckGroupAddMode 检查群聊加群方式
func (g *GroupService) CheckGroupAddMode(ctx context.Context, groupId string) (int8, error) {
	if g.cacheHelper == nil {
		group, err := g.uow.GroupStore().FindByUuid(ctx, groupId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return -1, errorx.New(errorx.CodeNotFound, "群组不存在")
			}
			return -1, err
		}
		return group.AddMode, nil
	}

	cacheKey := constants.CacheKeyGroupInfo + groupId
	var groupInfo GetGroupInfoRespond

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
			return GetGroupInfoRespond{
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
		cacheinfra.RandomizedTTL(24*time.Hour),
		5*time.Minute,
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
	member, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil
		}
		zap.L().Error("Check group membership error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if member.Role == 3 {
		return errorx.New(errorx.CodeInvalidParam, "群主不能退群，请先转让群主或解散群聊")
	}

	err = WithTx(g.uow, func(tx UoW) error {
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

			payload, _ := json.Marshal(event.GroupMemberLeftEvent{
				GroupId: groupId,
				UserId:  userId,
			})
			return tx.RecordEvent(ctx, groupId, event.EventGroupMemberLeft, payload)
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if g.cacheHelper != nil {
		_ = g.cacheHelper.InvalidateWithNull(ctx, constants.CacheKeyGroupInfo+groupId)
	}
	if g.cache != nil {
		_ = g.cache.Delete(ctx, constants.CacheKeyGroupMembers+groupId)
		g.cache.SubmitTask(func() {
			if g.cacheHelper != nil {
				_ = g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+groupId)
			}
			_ = g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+groupId)
		})
	}
	return nil
}

// DismissGroup 解散群聊 (operatorId 必须是群主)
func (g *GroupService) DismissGroup(ctx context.Context, operatorId, groupId string) error {
	group, err := g.uow.GroupStore().FindByUuid(ctx, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil
		}
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if group.OwnerId != operatorId {
		return errorx.New(errorx.CodeForbidden, "只有群主才能解散群聊")
	}

	err = WithTx(g.uow, func(tx UoW) error {
		if err := tx.GroupMemberStore().DeleteByGroupUuid(ctx, groupId); err != nil {
			zap.L().Error("Delete members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if err := tx.GroupStore().SoftDeleteByUuids(ctx, []string{groupId}); err != nil {
			zap.L().Error("Soft delete group error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		payload, _ := json.Marshal(event.GroupDismissedEvent{
			GroupId: groupId,
		})
		return tx.RecordEvent(ctx, groupId, event.EventGroupDismissed, payload)
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if g.cacheHelper != nil {
		_ = g.cacheHelper.InvalidateWithNull(ctx, constants.CacheKeyGroupInfo+groupId)
	}
	if g.cache != nil {
		_ = g.cache.Delete(ctx, constants.CacheKeyGroupMembers+groupId)
		g.cache.SubmitTask(func() {
			if g.cacheHelper != nil {
				_ = g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+groupId)
			}
			_ = g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+groupId)
		})
	}

	return nil
}

// UpdateGroupInfo 更新群组信息 (operatorId 必须是群主或管理员)
func (g *GroupService) UpdateGroupInfo(ctx context.Context, operatorId string, req UpdateGroupInfoRequest) error {
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

	if req.Version == nil {
		return errorx.New(errorx.CodeInvalidParam, "缺少版本号")
	}

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

	if err := WithTx(g.uow, func(tx UoW) error {
		return tx.GroupStore().UpdateWithVersion(ctx, req.Uuid, *req.Version, updates)
	}); err != nil {
		if errorx.IsCode(err, errorx.CodeConflict) {
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

	groupId := req.Uuid
	if g.cache != nil && g.cacheHelper != nil {
		g.cache.SubmitTask(func() {
			_ = g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+groupId)
		})
	}

	return nil
}

// GetGroupMemberList 获取群聊成员列表（分页）(userId 必须是群成员)
func (g *GroupService) GetGroupMemberList(ctx context.Context, userId, groupId string, page, pageSize int) ([]GetGroupMemberListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	_, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, 0, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	members, err := g.uow.GroupMemberStore().FindByGroupUuid(ctx, groupId)
	if err != nil {
		zap.L().Error("Find group members error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	total := int64(len(members))

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

	start := (page - 1) * pageSize
	if start >= len(members) {
		return []GetGroupMemberListRespond{}, total, nil
	}
	end := start + pageSize
	if end > len(members) {
		end = len(members)
	}

	rspList := make([]GetGroupMemberListRespond, 0, end-start)
	for _, m := range members[start:end] {
		u, ok := userMap[m.UserUuid]
		if !ok {
			continue
		}
		rspList = append(rspList, GetGroupMemberListRespond{
			UserId:   u.Uuid,
			Nickname: u.Nickname,
			Avatar:   u.Avatar,
		})
	}

	return rspList, total, nil
}

// RemoveGroupMembers 移除群聊成员
func (g *GroupService) RemoveGroupMembers(ctx context.Context, operatorId string, req RemoveGroupMembersRequest) error {
	if len(req.UuidList) == 0 {
		return nil
	}

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

	err = WithTx(g.uow, func(tx UoW) error {
		deletedCount, err := tx.GroupMemberStore().DeleteByUserUuids(ctx, req.GroupId, req.UuidList)
		if err != nil {
			zap.L().Error("Delete group members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if deletedCount > 0 {
			if err := tx.GroupStore().DecrementMemberCountBy(ctx, req.GroupId, int(deletedCount)); err != nil {
				zap.L().Error("Decrement member count error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			payload, _ := json.Marshal(event.GroupMemberRemovedEvent{
				GroupId:     req.GroupId,
				OperatorId:  operatorId,
				MemberUuids: req.UuidList,
			})
			return tx.RecordEvent(ctx, req.GroupId, event.EventGroupMemberRemoved, payload)
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if g.cacheHelper != nil {
		_ = g.cacheHelper.InvalidateWithNull(ctx, constants.CacheKeyGroupInfo+req.GroupId)
	}
	if g.cache != nil {
		_ = g.cache.Delete(ctx, constants.CacheKeyGroupMembers+req.GroupId)
		g.cache.SubmitTask(func() {
			if g.cacheHelper != nil {
				_ = g.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+req.GroupId)
			}
			_ = g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+req.GroupId)
		})
	}

	return nil
}

// GetGroupDetail 获取群聊详情 (userId 必须是群成员)
func (g *GroupService) GetGroupDetail(ctx context.Context, userId, groupId string) (PublicGroupInfoRespond, error) {
	if len(groupId) == 0 {
		return PublicGroupInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "群聊ID不能为空")
	}

	_, err := g.uow.GroupMemberStore().FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return PublicGroupInfoRespond{}, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Check group membership error", zap.Error(err))
		return PublicGroupInfoRespond{}, errorx.ErrServerBusy
	}

	if g.cacheHelper == nil {
		grp, err := g.uow.GroupStore().FindByUuid(ctx, groupId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return PublicGroupInfoRespond{}, errorx.New(errorx.CodeNotFound, "该群聊不存在")
			}
			return PublicGroupInfoRespond{}, err
		}
		if grp.Status == group_status.DISABLE {
			return PublicGroupInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "该群聊处于禁用状态")
		}
		return PublicGroupInfoRespond{
			GroupId:     grp.Uuid,
			GroupName:   grp.Name,
			GroupAvatar: grp.Avatar,
			GroupNotice: grp.Notice,
			MemberCnt:   grp.MemberCnt,
			OwnerId:     grp.OwnerId,
			AddMode:     grp.AddMode,
		}, nil
	}

	cacheKey := constants.CacheKeyGroupInfo + groupId
	var groupInfo GetGroupInfoRespond

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
			return GetGroupInfoRespond{
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
		cacheinfra.RandomizedTTL(time.Hour),
		5*time.Minute,
		&groupInfo,
	)
	if err != nil {
		return PublicGroupInfoRespond{}, err
	}

	return PublicGroupInfoRespond{
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
func (g *GroupService) MuteMember(ctx context.Context, operatorId string, req MuteMemberRequest) error {
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

	grp, err := g.uow.GroupStore().FindByUuid(ctx, req.GroupId)
	if err != nil {
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if grp.OwnerId == req.UserId {
		return errorx.New(errorx.CodeInvalidParam, "不能禁言群主")
	}

	_, err = g.uow.GroupMemberStore().FindByGroupAndUser(ctx, req.GroupId, req.UserId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "该用户不是群成员")
		}
		zap.L().Error("Find target member error", zap.Error(err))
		return errorx.ErrServerBusy
	}

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

	if g.cache != nil {
		g.cache.SubmitTask(func() {
			_ = g.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+req.GroupId)
		})
	}

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

// ListGroupMemberIds 返回群全部成员ID
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
	err := WithTx(g.uow, func(tx UoW) error {
		newMember := GroupMember{
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

	if g.cacheHelper != nil {
		_ = g.cacheHelper.InvalidateWithNull(ctx, constants.CacheKeyGroupInfo+groupId)
	}
	if g.cache != nil {
		_ = g.cache.Delete(ctx, constants.CacheKeyGroupMembers+groupId)
	}

	return nil
}
