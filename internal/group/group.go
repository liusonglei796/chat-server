package group

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/snowflake"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/group/group_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/event"
)

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

		err := database.ExecTx(c, g.db, func(txCtx context.Context) error {
			existing, err := g.groupStore.FindByOwnerIdAndName(txCtx, ownerId, newGroup.Name)
			if err == nil && existing != nil {
				return errorx.New(errorx.CodeInvalidParam, "您已拥有同名的正常群聊，请勿重复创建")
			}

			if err := g.groupStore.CreateGroup(txCtx, &newGroup); err != nil {
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
			if err := g.memberStore.CreateGroupMember(txCtx, &member); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			payload, _ := json.Marshal(event.GroupCreatedEvent{
				GroupId:     newGroup.Uuid,
				OwnerId:     ownerId,
				GroupName:   newGroup.Name,
				GroupAvatar: newGroup.Avatar,
			})
			return g.recordEvent(txCtx, newGroup.Uuid, event.EventGroupCreated, payload)
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

	groups, total, err := g.groupStore.FindByOwnerIdPaged(ctx, userId, page, pageSize)
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

	groupUuids, total, err := g.memberStore.FindGroupUuidsByUserPaged(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find group uuids by user paged error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	if len(groupUuids) == 0 {
		return []MyGroupListRespond{}, total, nil
	}

	groups, err := g.groupStore.FindByUuids(ctx, groupUuids)
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
		group, err := g.groupStore.FindByUuid(ctx, groupId)
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
			group, err := g.groupStore.FindByUuid(loaderCtx, groupId)
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

// DismissGroup 解散群聊 (operatorId 必须是群主)
func (g *GroupService) DismissGroup(ctx context.Context, operatorId, groupId string) error {
	group, err := g.groupStore.FindByUuid(ctx, groupId)
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
	if group.Status == group_status.DISMISS {
		return nil
	}

	err = database.ExecTx(ctx, g.db, func(txCtx context.Context) error {
		ok, err := g.groupStore.DismissGroupWithCAS(txCtx, groupId, operatorId)
		if err != nil {
			zap.L().Error("Dismiss group with CAS error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok {
			return nil
		}

		if err := g.memberStore.DeleteByGroupUuid(txCtx, groupId); err != nil {
			zap.L().Error("Delete members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		payload, _ := json.Marshal(event.GroupDismissedEvent{
			GroupId: groupId,
		})
		return g.recordEvent(txCtx, groupId, event.EventGroupDismissed, payload)
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
	member, err := g.memberStore.FindByGroupAndUser(ctx, req.Uuid, operatorId)
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

	if err := g.groupStore.UpdateWithVersion(ctx, req.Uuid, *req.Version, updates); err != nil {
		if errorx.IsCode(err, errorx.CodeConflict) {
			currentGroup, queryErr := g.groupStore.FindByUuid(ctx, req.Uuid)
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

// GetGroupDetail 获取群聊详情 (userId 必须是群成员)
func (g *GroupService) GetGroupDetail(ctx context.Context, userId, groupId string) (PublicGroupInfoRespond, error) {
	if len(groupId) == 0 {
		return PublicGroupInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "群聊ID不能为空")
	}

	_, err := g.memberStore.FindByGroupAndUser(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return PublicGroupInfoRespond{}, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Check group membership error", zap.Error(err))
		return PublicGroupInfoRespond{}, errorx.ErrServerBusy
	}

	if g.cacheHelper == nil {
		grp, err := g.groupStore.FindByUuid(ctx, groupId)
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
			grp, err := g.groupStore.FindByUuid(loaderCtx, groupId)
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
