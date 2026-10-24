package group

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/event"
)

// LeaveGroup 退群
func (g *GroupService) LeaveGroup(ctx context.Context, userId string, groupId string) error {
	member, err := g.memberStore.FindByGroupAndUser(ctx, groupId, userId)
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

	err = database.ExecTx(ctx, g.db, func(txCtx context.Context) error {
		ok, err := g.memberStore.DeleteMemberIfNotOwner(txCtx, groupId, userId)
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if ok {
			if err := g.groupStore.DecrementMemberCountBy(txCtx, groupId, 1); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			payload, _ := json.Marshal(event.GroupMemberLeftEvent{
				GroupId: groupId,
				UserId:  userId,
			})
			return g.recordOutboxEvent(txCtx, groupId, event.EventGroupMemberLeft, payload)
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

// GetGroupMemberList 获取群聊成员列表（分页）(userId 必须是群成员)
func (g *GroupService) GetGroupMemberList(ctx context.Context, userId, groupId string, page, pageSize int) ([]GetGroupMemberListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	_, err := g.memberStore.FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, 0, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	members, err := g.memberStore.FindByGroupUuid(ctx, groupId)
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

	member, err := g.memberStore.FindByGroupAndUser(ctx, req.GroupId, operatorId)
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

	group, err := g.groupStore.FindByUuid(ctx, req.GroupId)
	if err != nil {
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	for _, uuid := range req.UuidList {
		if group.OwnerId == uuid {
			return errorx.New(errorx.CodeInvalidParam, "不能移除群主")
		}
	}

	err = database.ExecTx(ctx, g.db, func(txCtx context.Context) error {
		deletedCount, err := g.memberStore.DeleteByUserUuids(txCtx, req.GroupId, req.UuidList)
		if err != nil {
			zap.L().Error("Delete group members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if deletedCount > 0 {
			if err := g.groupStore.DecrementMemberCountBy(txCtx, req.GroupId, int(deletedCount)); err != nil {
				zap.L().Error("Decrement member count error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			payload, _ := json.Marshal(event.GroupMemberRemovedEvent{
				GroupId:     req.GroupId,
				OperatorId:  operatorId,
				MemberUuids: req.UuidList,
			})
			return g.recordOutboxEvent(txCtx, req.GroupId, event.EventGroupMemberRemoved, payload)
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

// MuteMember 禁言/取消禁言群成员
func (g *GroupService) MuteMember(ctx context.Context, operatorId string, req MuteMemberRequest) error {
	operator, err := g.memberStore.FindByGroupAndUser(ctx, req.GroupId, operatorId)
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

	grp, err := g.groupStore.FindByUuid(ctx, req.GroupId)
	if err != nil {
		zap.L().Error("Find group error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if grp.OwnerId == req.UserId {
		return errorx.New(errorx.CodeInvalidParam, "不能禁言群主")
	}

	_, err = g.memberStore.FindByGroupAndUser(ctx, req.GroupId, req.UserId)
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

	if err := g.memberStore.UpdateMuteUntil(ctx, req.GroupId, req.UserId, muteUntil); err != nil {
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
	_, err := g.memberStore.FindByGroupAndUser(ctx, groupId, userId)
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
	member, err := g.memberStore.FindByGroupAndUser(ctx, groupId, userId)
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
	members, err := g.memberStore.FindByGroupUuid(ctx, groupId)
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

// AddGroupMember 跨服务向群组中添加成员（由 apply 审批通过后直调）
func (g *GroupService) AddGroupMember(ctx context.Context, groupId, userId string, role int8) error {
	err := database.ExecTx(ctx, g.db, func(txCtx context.Context) error {
		newMember := GroupMember{
			GroupUuid: groupId,
			UserUuid:  userId,
			Role:      role,
		}
		if err := g.memberStore.CreateGroupMember(txCtx, &newMember); err != nil {
			zap.L().Error("CreateGroupMember error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if err := g.groupStore.IncrementMemberCount(txCtx, groupId); err != nil {
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
