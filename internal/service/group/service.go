package group

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/infrastructure/snowflake"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/contact/contact_type_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/errorx"
)

// groupInfoService 群组业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖
type groupInfoService struct {
	repos *mysql.Repositories
	cache myredis.AsyncCacheService
}

// NewGroupService 构造函数，注入所有依赖
func NewGroupService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *groupInfoService {
	return &groupInfoService{
		repos: repos,
		cache: cacheService,
	}
}

// CreateGroup 创建群聊 (ownerId 从 JWT 获取)
func (g *groupInfoService) CreateGroup(ownerId string, groupReq request.CreateGroupRequest) error {
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
		// 添加联系人
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
		// 删除联系人关系缓存
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

// LoadMyGroup 获取我创建的群聊
func (g *groupInfoService) LoadMyGroup(userId string) ([]respond.MyGroupListRespond, error) {
	// 1. 获取所有关联群组
	allGroups, err := g.getGroupsByUserId(userId)
	if err != nil {
		return nil, err
	}

	// 2. 过滤出我创建的群组
	groupListRsp := make([]respond.MyGroupListRespond, 0)
	for _, grp := range allGroups {
		if grp.OwnerId == userId {
			groupListRsp = append(groupListRsp, respond.MyGroupListRespond{
				GroupId:   grp.Uuid,
				GroupName: grp.Name,
				Avatar:    grp.Avatar,
			})
		}
	}
	return groupListRsp, nil
}

// GetJoinedGroups 获取我加入的群组（包含自己创建的）
// 注意：如果业务层需要区分"我加入的" vs "我创建的"，可以在 Handler 层过滤，
// 或者复用 getGroupsByUserId 自行处理。
// 这里为了满足 Contact 模块的需求，通常是指"所有群组"或"非我创建的群组"，
// 之前的 Contact.GetJoinedGroupsExcludedOwn 是排除自己创建的。
// 我们这里提供 GetJoinedGroupsExcludedOwn 对应的逻辑：
func (g *groupInfoService) GetJoinedGroups(userId string) ([]respond.MyGroupListRespond, error) {
	// 1. 获取所有关联群组
	allGroups, err := g.getGroupsByUserId(userId)
	if err != nil {
		return nil, err
	}

	// 2. 过滤出我加入的群组 (排除自己创建的)
	groupListRsp := make([]respond.MyGroupListRespond, 0)
	for _, grp := range allGroups {
		if grp.OwnerId != userId {
			groupListRsp = append(groupListRsp, respond.MyGroupListRespond{
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
		contactList, dbErr := g.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP)
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
			// 注意：缓存里存的是 respond.GetGroupInfoRespond 结构，不是 model.GroupInfo
			// 所以我们需要 Unmarshal 到 DTO 然后转换，或者统一缓存结构。
			// 之前的代码是 Unmarshal 到 respond.GetGroupInfoRespond
			var dto respond.GetGroupInfoRespond
			if err := json.Unmarshal([]byte(val), &dto); err == nil {
				// DTO -> Model (部分字段)
				// 为了简化，我们只需要 ID, Name, Avatar, OwnerId
				result = append(result, model.GroupInfo{
					Uuid:    dto.Uuid,
					Name:    dto.Name,
					Avatar:  dto.Avatar,
					OwnerId: dto.OwnerId,
				})
				continue
			}
		}
		missingIds = append(missingIds, groupId)
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
				info := respond.GetGroupInfoRespond{
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
func (g *groupInfoService) CheckGroupAddMode(groupId string) (int8, error) {
	cacheKey := "group_info_" + groupId

	// 1. 尝试读取缓存
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var rsp respond.GetGroupInfoRespond
		// 如果反序列化成功，直接返回结果
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp.AddMode, nil
		}
		// 如果反序列化失败，记录日志，视为缓存脏数据，继续向下查库
		zap.L().Warn("Unmarshal group info cache failed, fallback to DB", zap.String("groupId", groupId), zap.Error(err))
	}

	// 2. 查询数据库 (Source of Truth)
	group, err := g.repos.Group.FindByUuid(groupId)
	if err != nil {
		zap.L().Error("Find group by uuid error", zap.Error(err))
		return -1, errorx.ErrServerBusy
	}

	// 3. 【关键】构建缓存对象
	cacheRsp := respond.GetGroupInfoRespond{
		Uuid:      group.Uuid,
		Name:      group.Name,
		Notice:    group.Notice,
		MemberCnt: group.MemberCnt,
		OwnerId:   group.OwnerId,
		AddMode:   group.AddMode,
		Status:    group.Status,
		Avatar:    group.Avatar,
		IsDeleted: false,
	}

	// 4. 异步回写缓存 (修复缓存)
	g.cache.SubmitTask(func() {
		rspBytes, err := json.Marshal(cacheRsp)
		if err != nil {
			zap.L().Error("Marshal group info for cache error", zap.Error(err))
			return
		}
		if err := g.cache.Set(context.Background(), cacheKey, string(rspBytes), time.Hour*24); err != nil {
			zap.L().Error("Set group info cache error", zap.Error(err))
		}
	})

	return group.AddMode, nil
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

		if err := txRepos.Contact.SoftDelete(userId, groupId); err != nil {
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

		// 5. 批量软删除涉及该群的联系人关系
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
// 类似于 UserService.GetPublicUserInfo，只返回公开字段
func (g *groupInfoService) GetPublicGroupInfo(groupId string) (*respond.PublicGroupInfoRespond, error) {
	cacheKey := "group_info_" + groupId

	// 1. 尝试从缓存获取
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var fullInfo respond.GetGroupInfoRespond
		if err := json.Unmarshal([]byte(rspString), &fullInfo); err == nil {
			// 转换为公开信息（只包含非敏感字段）
			return &respond.PublicGroupInfoRespond{
				Uuid:      fullInfo.Uuid,
				Name:      fullInfo.Name,
				Notice:    fullInfo.Notice,
				Avatar:    fullInfo.Avatar,
				MemberCnt: fullInfo.MemberCnt,
				AddMode:   fullInfo.AddMode,
			}, nil
		}
		zap.L().Warn("Unmarshal group info cache failed", zap.String("groupId", groupId), zap.Error(err))
	}

	// 2. 查询数据库
	group, err := g.repos.Group.FindByUuid(groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, errorx.New(errorx.CodeNotFound, "群组不存在")
		}
		zap.L().Error("Find group error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 3. 构建公开响应（不包含 status, is_deleted, owner_id 等敏感信息）
	rsp := &respond.PublicGroupInfoRespond{
		Uuid:      group.Uuid,
		Name:      group.Name,
		Notice:    group.Notice,
		Avatar:    group.Avatar,
		MemberCnt: group.MemberCnt,
		AddMode:   group.AddMode,
	}

	// 4. 异步回写完整缓存（供其他需要完整信息的方法使用）
	cacheGroup := *group
	g.cache.SubmitTask(func() {
		info := respond.GetGroupInfoRespond{
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
			_ = g.cache.Set(context.Background(), cacheKey, string(data), time.Hour*24)
		}
	})

	return rsp, nil
}

// UpdateGroupInfo 更新群组信息 (operatorId 必须是群主或管理员)
func (g *groupInfoService) UpdateGroupInfo(operatorId string, req request.UpdateGroupInfoRequest) error {
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

	// 异步清理缓存
	g.cache.SubmitTask(func() {
		if err := g.cache.Delete(context.Background(), "group_info_"+req.Uuid); err != nil {
			zap.L().Error("service error", zap.Error(err))
		}
	})

	return nil
}

// GetGroupMemberList 获取群聊成员列表 (userId 必须是群成员)
func (g *groupInfoService) GetGroupMemberList(userId, groupId string) ([]respond.GetGroupMemberListRespond, error) {
	// 权限校验: 必须是群成员
	_, err := g.repos.GroupMember.FindByGroupAndUser(groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	cacheKey := "group_memberlist_" + groupId

	// 1. 尝试从缓存获取
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var rsp []respond.GetGroupMemberListRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp, nil
		}
		// 反序列化失败，记录警告并降级查库
		zap.L().Warn("Unmarshal group member list cache failed", zap.String("groupId", groupId), zap.Error(err))
	} else if err != nil {
		// Redis 异常（非 Key 不存在），记录错误并降级查库
		zap.L().Error("Get group member list cache error", zap.String("groupId", groupId), zap.Error(err))
	}

	// 2. 查询数据库
	members, err := g.repos.GroupMember.FindMembersWithUserInfo(groupId)
	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 3. 构建响应 (预分配)
	rspList := make([]respond.GetGroupMemberListRespond, 0, len(members))
	for _, m := range members {
		rspList = append(rspList, respond.GetGroupMemberListRespond{
			UserId:   m.UserId,
			Nickname: m.Nickname,
			Avatar:   m.Avatar,
		})
	}

	// 4. 回写缓存 (异步)
	g.cache.SubmitTask(func() {
		data, err := json.Marshal(rspList)
		if err != nil {
			zap.L().Error("Marshal group member list error", zap.Error(err))
			return
		}
		if err := g.cache.Set(context.Background(), cacheKey, string(data), time.Hour*24); err != nil {
			zap.L().Error("Set group member list cache error", zap.Error(err))
		}
	})

	return rspList, nil
}

// RemoveGroupMembers 移除群聊成员 (operatorId 必须是群主或管理员)
func (g *groupInfoService) RemoveGroupMembers(operatorId string, req request.RemoveGroupMembersRequest) error {
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
			if err := txRepos.Contact.SoftDelete(uuid, req.GroupId); err != nil {
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
