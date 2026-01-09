package group

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql/repository"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/contact/contact_type_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/random"
)

// groupInfoService 群组业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖
type groupInfoService struct {
	repos *repository.Repositories
	cache myredis.AsyncCacheService
}

// NewGroupService 构造函数，注入所有依赖
func NewGroupService(repos *repository.Repositories, cacheService myredis.AsyncCacheService) *groupInfoService {
	return &groupInfoService{
		repos: repos,
		cache: cacheService,
	}
}

// CreateGroup 创建群聊
func (g *groupInfoService) CreateGroup(groupReq request.CreateGroupRequest) error {
	group := model.GroupInfo{
		Uuid:      fmt.Sprintf("G%s", random.GetNowAndLenRandomString(11)),
		Name:      groupReq.Name,
		Notice:    groupReq.Notice,
		OwnerId:   groupReq.OwnerId,
		MemberCnt: 1,
		AddMode:   groupReq.AddMode,
		Avatar:    groupReq.Avatar,
		Status:    group_status_enum.NORMAL,
	}

	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		if err := txRepos.Group.CreateGroup(&group); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}
		// 创建群成员
		member := model.GroupMember{
			GroupUuid: group.Uuid,
			UserUuid:  groupReq.OwnerId,
			Role:      3,
		}
		if err := txRepos.GroupMember.CreateGroupMember(&member); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}
		// 添加联系人
		contact := model.Contact{
			UserId:      groupReq.OwnerId,
			ContactId:   group.Uuid,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&contact); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	g.cache.SubmitTask(func() {
		if err := g.cache.DeleteByPattern(context.Background(), "contact_mygroup_list_"+groupReq.OwnerId+"*"); err != nil {
			zap.L().Error(err.Error())
		}
	})

	return nil
}

// LoadMyGroup 获取我创建的群聊
func (g *groupInfoService) LoadMyGroup(userId string) ([]respond.LoadMyGroupRespond, error) {
	cacheKey := "contact_mygroup_list_" + userId

	// 1. 尝试从缓存获取 (Happy Path)
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var groupListRsp []respond.LoadMyGroupRespond
		// 如果反序列化成功，直接返回
		if err := json.Unmarshal([]byte(rspString), &groupListRsp); err == nil {
			return groupListRsp, nil
		}
		// 如果反序列化失败（缓存数据脏了），打个日志，继续往下走查数据库
		zap.L().Error("Unmarshal my group list cache error", zap.Error(err))
	} else if err != nil {
		// 如果是 Redis 连接错误等非"Key不存在"的错误，记录日志但不中断业务
		zap.L().Error("Redis get error", zap.Error(err))
	}

	// 2. 缓存未命中 或 缓存出错 -> 查询数据库
	groupList, err := g.repos.Group.FindByOwnerId(userId)
	if err != nil {
		zap.L().Error("Find my groups from DB error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 3. 构建返回结果
	// 使用 make 初始化 len=0，确保序列化后是 [] 而不是 null
	groupListRsp := make([]respond.LoadMyGroupRespond, 0, len(groupList))
	for _, group := range groupList {
		groupListRsp = append(groupListRsp, respond.LoadMyGroupRespond{
			GroupId:   group.Uuid,
			GroupName: group.Name,
			Avatar:    group.Avatar,
		})
	}

	// 4. 回写缓存 (异步)
	g.cache.SubmitTask(func() {
		rspBytes, err := json.Marshal(groupListRsp)
		if err == nil {
			if err := g.cache.Set(context.Background(), cacheKey, string(rspBytes), time.Minute*30); err != nil {
				zap.L().Error("Set cache error", zap.Error(err))
			}
		} else {
			zap.L().Error("Marshal group list error", zap.Error(err))
		}
	})

	return groupListRsp, nil
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

// EnterGroupDirectly 直接进群
func (g *groupInfoService) EnterGroupDirectly(groupId, userId string) error {
	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		member := model.GroupMember{
			GroupUuid: groupId,
			UserUuid:  userId,
			Role:      1,
		}
		if err := txRepos.GroupMember.CreateGroupMember(&member); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}

		if err := txRepos.Group.IncrementMemberCount(groupId); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}

		newContact := model.Contact{
			UserId:      userId,
			ContactId:   groupId,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&newContact); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	g.cache.SubmitTask(func() {
		if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+groupId+"*"); err != nil {
			zap.L().Error(err.Error())
		}
		if err := g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+userId+"*"); err != nil {
			zap.L().Error(err.Error())
		}
		if err := g.cache.Delete(context.Background(), "group_info_"+groupId); err != nil {
			zap.L().Error(err.Error())
		}
	})
	return nil
}

// LeaveGroup 退群
func (g *groupInfoService) LeaveGroup(userId string, groupId string) error {
	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		if err := txRepos.GroupMember.DeleteByUserUuids(groupId, []string{userId}); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}

		if err := txRepos.Group.DecrementMemberCountBy(groupId, 1); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}

		session, _ := txRepos.Session.FindBySendIdAndReceiveId(userId, groupId)
		if session != nil {
			if err := txRepos.Session.SoftDeleteByUuids([]string{session.Uuid}); err != nil {
				zap.L().Error(err.Error())
			}
		}

		if err := txRepos.Contact.SoftDelete(userId, groupId); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}
		if err := txRepos.Apply.SoftDelete(userId, groupId); err != nil {
			zap.L().Error(err.Error())
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	g.cache.SubmitTask(func() {
		if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+userId+"*"); err != nil {
			zap.L().Error(err.Error())
		}
		if err := g.cache.RemoveFromSet(context.Background(), "contact_relation:group:"+userId, groupId); err != nil {
			zap.L().Error(err.Error())
		}
		if err := g.cache.Delete(context.Background(), "group_info_"+groupId); err != nil {
			zap.L().Error(err.Error())
		}
		if err := g.cache.Delete(context.Background(), "group_memberlist_"+groupId); err != nil {
			zap.L().Error(err.Error())
		}
	})
	return nil
}

// DismissGroup 解散群聊
func (g *groupInfoService) DismissGroup(ownerId, groupId string) error {
	var memberIds []string

	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 获取涉及的成员ID
		contacts, err := txRepos.Contact.FindUsersByContactId(groupId)
		if err != nil {
			zap.L().Error("Find contacts by group id error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		for _, c := range contacts {
			memberIds = append(memberIds, c.UserId)
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
		return err
	}

	// 7. 精确清理 Redis 缓存 (事务外)
	g.cache.SubmitTask(func() {
		// 清理群主的缓存
		if err := g.cache.DeleteByPattern(context.Background(), "contact_mygroup_list_"+ownerId+"*"); err != nil {
			zap.L().Error(err.Error())
		}
		if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+ownerId+"*"); err != nil {
			zap.L().Error(err.Error())
		}

		// 清理所有群成员的缓存
		for _, memberId := range memberIds {
			if err := g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memberId+"*"); err != nil {
				zap.L().Error(err.Error())
			}
			if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memberId+"*"); err != nil {
				zap.L().Error(err.Error())
			}
		}

		// 清理群公共信息
		if err := g.cache.Delete(context.Background(), "group_info_"+groupId); err != nil {
			zap.L().Error(err.Error())
		}
		if err := g.cache.Delete(context.Background(), "group_memberlist_"+groupId); err != nil {
			zap.L().Error(err.Error())
		}
	})

	return nil
}

// GetGroupInfo 获取群聊详情
func (g *groupInfoService) GetGroupInfo(groupId string) (*respond.GetGroupInfoRespond, error) {
	cacheKey := "group_info_" + groupId

	// 1. 尝试从缓存获取
	rspString, err := g.cache.Get(context.Background(), cacheKey)
	if err == nil && rspString != "" {
		var rsp respond.GetGroupInfoRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return &rsp, nil
		}
		// 反序列化失败，记录警告并降级查库
		zap.L().Warn("Unmarshal group info cache failed", zap.String("groupId", groupId), zap.Error(err))
	} else if err != nil {
		// Redis 异常（非 Key 不存在），记录错误并降级查库
		zap.L().Error("Get group info cache error", zap.String("groupId", groupId), zap.Error(err))
	}

	// 2. 查询数据库
	group, err := g.repos.Group.FindByUuid(groupId)
	if err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}

	// 3. 构建响应
	rsp := &respond.GetGroupInfoRespond{
		Uuid:      group.Uuid,
		Name:      group.Name,
		Notice:    group.Notice,
		Avatar:    group.Avatar,
		MemberCnt: group.MemberCnt,
		OwnerId:   group.OwnerId,
		AddMode:   group.AddMode,
		Status:    group.Status,
	}
	if group.DeletedAt.Valid {
		rsp.IsDeleted = true
	} else {
		rsp.IsDeleted = false
	}

	// 4. 回写缓存 (异步)
	g.cache.SubmitTask(func() {
		data, err := json.Marshal(rsp)
		if err != nil {
			zap.L().Error("Marshal group info error", zap.Error(err))
			return
		}
		if err := g.cache.Set(context.Background(), cacheKey, string(data), time.Hour*24); err != nil {
			zap.L().Error("Set group info cache error", zap.Error(err))
		}
	})

	return rsp, nil
}

// GetGroupInfoList 获取群聊列表 - 管理员
func (g *groupInfoService) GetGroupInfoList(req request.GetGroupListRequest) (*respond.GetGroupListWrapper, error) {
	groupList, total, err := g.repos.Group.GetGroupList(req.Page, req.PageSize)
	if err != nil {
		zap.L().Error(err.Error())
		return nil, errorx.ErrServerBusy
	}
	rsp := make([]respond.GetGroupListRespond, 0, len(groupList))
	for _, group := range groupList {
		rp := respond.GetGroupListRespond{
			Uuid:      group.Uuid,
			Name:      group.Name,
			OwnerId:   group.OwnerId,
			Status:    group.Status,
			IsDeleted: group.DeletedAt.Valid,
		}
		rsp = append(rsp, rp)
	}
	return &respond.GetGroupListWrapper{
		List:  rsp,
		Total: total,
	}, nil
}

// DeleteGroups 删除列表中群聊 - 管理员
func (g *groupInfoService) DeleteGroups(uuidList []string) error {
	if len(uuidList) == 0 {
		return nil
	}

	// 1. 准备工作：收集需要清理缓存的用户ID
	groups, err := g.repos.Group.FindByUuids(uuidList)
	if err != nil {
		zap.L().Error("Find groups error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	ownerIds := make([]string, 0, len(groups))
	for _, grp := range groups {
		ownerIds = append(ownerIds, grp.OwnerId)
	}

	// 查出涉事群组的所有成员ID
	memberIds, err := g.repos.GroupMember.GetMemberIdsByGroupUuids(uuidList)
	if err != nil {
		zap.L().Error("Find group members error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 事务执行删除操作
	err = g.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 删除群成员 (Batch)
		if err := txRepos.GroupMember.DeleteByGroupUuids(uuidList); err != nil {
			zap.L().Error("Batch delete group members error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除群组
		if err := txRepos.Group.SoftDeleteByUuids(uuidList); err != nil {
			zap.L().Error("Batch soft delete groups error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除相关会话
		if err := txRepos.Session.SoftDeleteByUsers(uuidList); err != nil {
			zap.L().Error("Batch soft delete sessions error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除相关联系人
		if err := txRepos.Contact.SoftDeleteByUsers(uuidList); err != nil {
			zap.L().Error("Batch soft delete contacts error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 软删除相关申请
		if err := txRepos.Apply.SoftDeleteByUsers(uuidList); err != nil {
			zap.L().Error("Batch soft delete contact applies error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步清理缓存
	g.cache.SubmitTask(func() {
		// 清理群主相关缓存
		for _, ownerId := range ownerIds {
			if err := g.cache.DeleteByPattern(context.Background(), "contact_mygroup_list_"+ownerId+"*"); err != nil {
				zap.L().Error(err.Error())
			}
			if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+ownerId+"*"); err != nil {
				zap.L().Error(err.Error())
			}
		}

		// 清理所有相关成员的缓存
		for _, memId := range memberIds {
			if err := g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memId+"*"); err != nil {
				zap.L().Error(err.Error())
			}
			if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memId+"*"); err != nil {
				zap.L().Error(err.Error())
			}
		}

		// 清理群本身的缓存
		for _, grpId := range uuidList {
			if err := g.cache.Delete(context.Background(), "group_info_"+grpId); err != nil {
				zap.L().Error(err.Error())
			}
			if err := g.cache.Delete(context.Background(), "group_memberlist_"+grpId); err != nil {
				zap.L().Error(err.Error())
			}
		}
	})

	return nil
}

// SetGroupsStatus 设置群聊是否启用
func (g *groupInfoService) SetGroupsStatus(uuidList []string, status int8) error {
	if len(uuidList) == 0 {
		return nil
	}

	if err := g.repos.Group.UpdateStatusByUuids(uuidList, status); err != nil {
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}

	if status == group_status_enum.DISABLE {
		if err := g.repos.Session.SoftDeleteByUsers(uuidList); err != nil {
			zap.L().Error(err.Error())
		}
	}

	g.cache.SubmitTask(func() {
		var patterns []string
		for _, uuid := range uuidList {
			patterns = append(patterns, "group_info_"+uuid)
		}
		if err := g.cache.DeleteByPatterns(context.Background(), patterns); err != nil {
			zap.L().Error(err.Error())
		}
	})

	return nil
}

// UpdateGroupInfo 更新群聊消息
func (g *groupInfoService) UpdateGroupInfo(req request.UpdateGroupInfoRequest) error {
	group, err := g.repos.Group.FindByUuid(req.Uuid)
	if err != nil {
		zap.L().Error(err.Error())
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
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}

	// 批量更新 Session
	sessionUpdates := map[string]interface{}{
		"receive_name": group.Name,
		"avatar":       group.Avatar,
	}
	if err := g.repos.Session.UpdateByReceiveId(req.Uuid, sessionUpdates); err != nil {
		zap.L().Error(err.Error())
	}

	// 异步清理缓存
	g.cache.SubmitTask(func() {
		if err := g.cache.Delete(context.Background(), "group_info_"+req.Uuid); err != nil {
			zap.L().Error(err.Error())
		}
	})

	return nil
}

// GetGroupMemberList 获取群聊成员列表
func (g *groupInfoService) GetGroupMemberList(groupId string) ([]respond.GetGroupMemberListRespond, error) {
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
		zap.L().Error(err.Error())
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

// RemoveGroupMembers 移除群聊成员
func (g *groupInfoService) RemoveGroupMembers(req request.RemoveGroupMembersRequest) error {
	if len(req.UuidList) == 0 {
		return nil
	}

	// 1. 校验参数：不允许移除群主
	for _, uuid := range req.UuidList {
		if req.OwnerId == uuid {
			return errorx.New(errorx.CodeInvalidParam, "不能移除群主")
		}
	}

	// 2. 事务执行删除操作
	err := g.repos.Transaction(func(txRepos *repository.Repositories) error {
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

		// 软删除 Session
		if err := txRepos.Session.SoftDeleteByUsers([]string{req.GroupId}); err != nil {
			zap.L().Error("Delete sessions error", zap.Error(err))
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步精确清理缓存
	g.cache.SubmitTask(func() {
		// 清理被移除成员的缓存
		for _, memId := range req.UuidList {
			if err := g.cache.DeleteByPattern(context.Background(), "group_session_list_"+memId+"*"); err != nil {
				zap.L().Error(err.Error())
			}
			if err := g.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+memId+"*"); err != nil {
				zap.L().Error(err.Error())
			}
		}
		// 清理群本身的缓存
		if err := g.cache.Delete(context.Background(), "group_info_"+req.GroupId); err != nil {
			zap.L().Error(err.Error())
		}
		if err := g.cache.Delete(context.Background(), "group_memberlist_"+req.GroupId); err != nil {
			zap.L().Error(err.Error())
		}
	})

	return nil
}
