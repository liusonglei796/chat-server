package contact

import (
	"encoding/json"

	"fmt"

	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql/repository"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/request"
	"kama_chat_server/internal/dto/respond"
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/contact/contact_type_enum"
	"kama_chat_server/pkg/enum/contact_apply/contact_apply_status_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/random"
)

// userContactService 联系人业务逻辑实现
type userContactService struct {
	repos *repository.Repositories
}

// NewContactService 构造函数
func NewContactService(repos *repository.Repositories) *userContactService {
	return &userContactService{repos: repos}
}

// GetUserList 获取指定用户的“好友（联系人）的用户信息列表”。
func (u *userContactService) GetUserList(userId string) ([]respond.MyUserListRespond, error) {
	// Optimization: Use Redis Set to store Friend IDs (contact_relation:user:<uid>)
	// This avoids storing huge JSON lists and ensures data consistency with UserInfo cache.
	cacheKey := "contact_relation:user:" + userId

	// 1. Try to get Member IDs from Redis
	memberIds, err := myredis.SMembers(cacheKey)
	if err != nil || len(memberIds) == 0 {
		// 2. Cache Miss or Empty: Fetch from DB
		contactList, dbErr := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.USER)
		if dbErr != nil {
			zap.L().Error("Find contact list error", zap.Error(dbErr))
			return nil, errorx.ErrServerBusy
		}

		// Re-populate memberIds
		memberIds = make([]string, 0, len(contactList))
		for _, c := range contactList {
			memberIds = append(memberIds, c.ContactId)
		}

		// Write back to Redis (If not empty)
		if len(memberIds) > 0 {
			membersArgs := make([]interface{}, len(memberIds))
			for i, v := range memberIds {
				membersArgs[i] = v
			}
			// Set expiration (e.g., 24 hours) - Set operations usually don't support EX in one command easily without pipeline,
			// but we can use generic Expand or just let it persist and ensure invalidation works.
			// Here we just SAdd.
			_ = myredis.SAdd(cacheKey, membersArgs...)
			// Optional: Set expiration if needed.
		}
	}

	if len(memberIds) == 0 {
		return []respond.MyUserListRespond{}, nil
	}

	// 3. Batch fetch User Info (Source of Truth or User Cache)
	// Ideally we should MGET "user_info:<id>" from Redis first, then fallback to DB.
	// For simplicity and consistency, we use the Repo's FindByUuids which typically queries DB.
	// If performance is critical, Repos should handle the caching of entities.
	users, err := u.repos.User.FindByUuids(memberIds)
	if err != nil {
		zap.L().Error("Batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 4. Assemble Response
	userListRsp := make([]respond.MyUserListRespond, 0, len(users))
	for _, user := range users {
		userListRsp = append(userListRsp, respond.MyUserListRespond{
			UserId:   user.Uuid,
			UserName: user.Nickname,
			Avatar:   user.Avatar,
		})
	}

	return userListRsp, nil
}

// 辅助方法：统一设置缓存 (已废弃，保留兼容性或用于其他简单Key)
func (u *userContactService) setCache(key string, data interface{}) {
	rspBytes, err := json.Marshal(data)
	if err != nil {
		zap.L().Error("Marshal cache error", zap.Error(err), zap.String("key", key))
		return
	}
	_ = myredis.SetKeyEx(key, string(rspBytes), time.Minute*constants.REDIS_TIMEOUT)
}

// GetJoinedGroupsExcludedOwn 获取我加入的群组列表（不包含自己创建的）
// Renamed from LoadMyJoinedGroup to clarify logic.
func (u *userContactService) GetJoinedGroupsExcludedOwn(userId string) ([]respond.LoadMyJoinedGroupRespond, error) {
	// Optimization: Use Redis Set for Group IDs
	cacheKey := "contact_relation:group:" + userId

	// 1. Try to get Group IDs from Redis
	groupUuids, err := myredis.SMembers(cacheKey)
	if err != nil || len(groupUuids) == 0 {
		// 2. Cache Miss: Fetch from DB
		contactList, dbErr := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP)
		if dbErr != nil {
			zap.L().Error("Find joined groups error", zap.Error(dbErr))
			return nil, errorx.ErrServerBusy
		}

		// Filter IDs (Must safeguard against non-G prefixes just in case)
		groupUuids = make([]string, 0, len(contactList))
		for _, contact := range contactList {
			if len(contact.ContactId) > 0 && contact.ContactId[0] == 'G' {
				groupUuids = append(groupUuids, contact.ContactId)
			}
		}

		// Write back to Redis
		if len(groupUuids) > 0 {
			args := make([]interface{}, len(groupUuids))
			for i, v := range groupUuids {
				args[i] = v
			}
			_ = myredis.SAdd(cacheKey, args...)
		}
	}

	if len(groupUuids) == 0 {
		return []respond.LoadMyJoinedGroupRespond{}, nil
	}

	// 3. Batch fetch Group Info
	groups, err := u.repos.Group.FindByUuids(groupUuids)
	if err != nil {
		zap.L().Error("Batch find groups error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 4. Assemble Response (Filter OwnerId here to be safe and strictly adhere to "ExcludedOwn")
	// Although the Redis Set *should* theoretically only contain valid joined groups,
	// enforcing the filter logic ensures consistency.
	groupListRsp := make([]respond.LoadMyJoinedGroupRespond, 0, len(groups))
	for _, group := range groups {
		if group.OwnerId != userId {
			groupListRsp = append(groupListRsp, respond.LoadMyJoinedGroupRespond{
				GroupId:   group.Uuid,
				GroupName: group.Name,
				Avatar:    group.Avatar,
			})
		}
	}

	return groupListRsp, nil
}

// GetFriendInfo 获取好友详情
func (u *userContactService) GetFriendInfo(friendId string) (respond.GetFriendInfoRespond, error) {
	// 1. 安全检查
	if len(friendId) == 0 {
		return respond.GetFriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	// 2. 尝试从缓存获取
	cacheKey := "user_info_" + friendId
	cachedStr, err := myredis.GetKey(cacheKey)
	if err == nil && cachedStr != "" {
		var userRsp respond.GetUserInfoRespond
		if err := json.Unmarshal([]byte(cachedStr), &userRsp); err == nil {
			return respond.GetFriendInfoRespond{
				FriendId:        userRsp.Uuid,
				FriendName:      userRsp.Nickname,
				FriendAvatar:    userRsp.Avatar,
				FriendBirthday:  userRsp.Birthday,
				FriendEmail:     userRsp.Email,
				FriendPhone:     userRsp.Telephone,
				FriendGender:    userRsp.Gender,
				FriendSignature: userRsp.Signature,
			}, nil
		}
		zap.L().Error("Unmarshal user info cache error", zap.Error(err), zap.String("cacheKey", cacheKey))
	}

	// 3. 缓存未命中，从数据库查询
	user, err := u.repos.User.FindByUuid(friendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return respond.GetFriendInfoRespond{}, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		zap.L().Error("Find user error", zap.Error(err), zap.String("friendId", friendId))
		return respond.GetFriendInfoRespond{}, errorx.ErrServerBusy
	}

	// 4. 检查用户状态
	if user.Status == user_status_enum.DISABLE {
		return respond.GetFriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "该用户处于禁用状态")
	}

	rsp := respond.GetFriendInfoRespond{
		FriendId:        user.Uuid,
		FriendName:      user.Nickname,
		FriendAvatar:    user.Avatar,
		FriendBirthday:  user.Birthday,
		FriendEmail:     user.Email,
		FriendPhone:     user.Telephone,
		FriendGender:    user.Gender,
		FriendSignature: user.Signature,
	}

	// 5. 回写缓存
	userRsp := respond.GetUserInfoRespond{
		Uuid:      user.Uuid,
		Telephone: user.Telephone,
		Nickname:  user.Nickname,
		Avatar:    user.Avatar,
		Birthday:  user.Birthday,
		Email:     user.Email,
		Gender:    user.Gender,
		Signature: user.Signature,
		CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		IsAdmin:   user.IsAdmin,
		Status:    user.Status,
	}
	if data, err := json.Marshal(userRsp); err == nil {
		_ = myredis.SetKeyEx(cacheKey, string(data), time.Hour)
	}

	return rsp, nil
}

// GetGroupDetail 获取群聊详情
func (u *userContactService) GetGroupDetail(groupId string) (respond.GetGroupDetailRespond, error) {
	// 1. 安全检查
	if len(groupId) == 0 {
		return respond.GetGroupDetailRespond{}, errorx.New(errorx.CodeInvalidParam, "群聊ID不能为空")
	}

	// 2. 尝试从缓存获取
	cacheKey := "group_info_" + groupId
	cachedStr, err := myredis.GetKey(cacheKey)
	if err == nil && cachedStr != "" {
		var groupRsp respond.GetGroupInfoRespond
		if err := json.Unmarshal([]byte(cachedStr), &groupRsp); err == nil {
			return respond.GetGroupDetailRespond{
				GroupId:     groupRsp.Uuid,
				GroupName:   groupRsp.Name,
				GroupAvatar: groupRsp.Avatar,
				GroupNotice: groupRsp.Notice,
				MemberCnt:   groupRsp.MemberCnt,
				OwnerId:     groupRsp.OwnerId,
				AddMode:     groupRsp.AddMode,
			}, nil
		}
		zap.L().Error("Unmarshal group info cache error", zap.Error(err), zap.String("cacheKey", cacheKey))
	}

	// 3. 缓存未命中，从数据库查询
	group, err := u.repos.Group.FindByUuid(groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return respond.GetGroupDetailRespond{}, errorx.New(errorx.CodeNotFound, "该群聊不存在")
		}
		zap.L().Error("Find group error", zap.Error(err), zap.String("groupId", groupId))
		return respond.GetGroupDetailRespond{}, errorx.ErrServerBusy
	}

	// 4. 检查群组状态
	if group.Status == group_status_enum.DISABLE {
		return respond.GetGroupDetailRespond{}, errorx.New(errorx.CodeInvalidParam, "该群聊处于禁用状态")
	}

	rsp := respond.GetGroupDetailRespond{
		GroupId:     group.Uuid,
		GroupName:   group.Name,
		GroupAvatar: group.Avatar,
		GroupNotice: group.Notice,
		MemberCnt:   group.MemberCnt,
		OwnerId:     group.OwnerId,
		AddMode:     group.AddMode,
	}

	// 5. 回写缓存
	groupRsp := respond.GetGroupInfoRespond{
		Uuid:      group.Uuid,
		Name:      group.Name,
		Notice:    group.Notice,
		Avatar:    group.Avatar,
		MemberCnt: group.MemberCnt,
		OwnerId:   group.OwnerId,
		AddMode:   group.AddMode,
		Status:    group.Status,
		IsDeleted: group.DeletedAt.Valid,
	}
	if data, err := json.Marshal(groupRsp); err == nil {
		_ = myredis.SetKeyEx(cacheKey, string(data), time.Hour)
	}

	return rsp, nil
}

// DeleteContact 删除联系人
func (u *userContactService) DeleteContact(userId, contactId string) error {
	// 使用事务确保操作原子性
	err := u.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 仅从“我的”联系人列表中移除对方 (单向操作)
		if err := txRepos.Contact.SoftDelete(userId, contactId); err != nil {
			zap.L().Error("Delete contact relation error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 2. 仅清理“我的”视角下的会话 (Session)
		// 先找到这个特定的会话
		session, err := txRepos.Session.FindBySendIdAndReceiveId(userId, contactId)
		if err == nil && session != nil {
			// 仅删除这一个会话记录
			if err := txRepos.Session.SoftDeleteByUuids([]string{session.Uuid}); err != nil {
				zap.L().Error("Delete session error", zap.Error(err))
				return errorx.ErrServerBusy
			}
		}

		// 3. 清理“我的”视角下的申请记录 (可选，通常为了防止再次申请时逻辑混淆)
		_ = txRepos.ContactApply.SoftDelete(userId, contactId)

		return nil
	})

	if err != nil {
		return err
	}

	// 4. 异步清理"我的"缓存
	myredis.SubmitCacheTask(func() {
		_ = myredis.DelKeysWithPattern("contact_user_list_" + userId)
		_ = myredis.DelKeysWithPattern("direct_session_list_" + userId)
	})

	return nil
}

// ApplyFriend 申请添加好友
func (u *userContactService) ApplyFriend(req request.ApplyFriendRequest) error {
	// 1. 安全检查
	if len(req.FriendId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	// 2. 校验目标用户是否存在且有效
	user, err := u.repos.User.FindByUuid(req.FriendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		return errorx.ErrServerBusy
	}
	if user.Status == user_status_enum.DISABLE {
		return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
	}

	// 3. 检查是否已经是好友，防止重复操作
	relation, err := u.repos.Contact.FindByUserIdAndContactId(req.UserId, req.FriendId)
	if err == nil && relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你们已经是好友")
	}

	// 4. 获取或创建申请记录
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(req.UserId, req.FriendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			// 第一次申请，创建新记录
			contactApply = &model.ContactApply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: req.UserId,
				TargetId:    req.FriendId,
				ContactType: contact_type_enum.USER,
				Status:      contact_apply_status_enum.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
			}
			if err := u.repos.ContactApply.Create(contactApply); err != nil {
				zap.L().Error("Create friend apply error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			return nil
		}
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 5. 黑名单校验
	if contactApply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑，无法发送申请")
	}

	// 6. 更新旧记录
	contactApply.LastApplyAt = time.Now()
	contactApply.Status = contact_apply_status_enum.PENDING
	contactApply.Message = req.Message

	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error("Update friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// ApplyGroup 申请加入群组
func (u *userContactService) ApplyGroup(req request.ApplyGroupRequest) error {
	// 1. 安全检查
	if len(req.GroupId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "群组ID不能为空")
	}

	// 2. 校验目标群组是否存在且有效
	group, err := u.repos.Group.FindByUuid(req.GroupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "该群聊不存在")
		}
		return errorx.ErrServerBusy
	}
	if group.Status == group_status_enum.DISABLE {
		return errorx.New(errorx.CodeInvalidParam, "该群聊已被禁用")
	}

	// 3. 检查是否已在群中，防止重复操作
	relation, err := u.repos.Contact.FindByUserIdAndContactId(req.UserId, req.GroupId)
	if err == nil && relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你已在该群中")
	}

	// 4. 获取或创建申请记录
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(req.UserId, req.GroupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			// 第一次申请，创建新记录
			contactApply = &model.ContactApply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: req.UserId,
				TargetId:    req.GroupId,
				ContactType: contact_type_enum.GROUP,
				Status:      contact_apply_status_enum.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
			}
			if err := u.repos.ContactApply.Create(contactApply); err != nil {
				zap.L().Error("Create group apply error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			return nil
		}
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 5. 黑名单校验
	if contactApply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "该群已将你拉黑，无法发送申请")
	}

	// 6. 更新旧记录
	contactApply.LastApplyAt = time.Now()
	contactApply.Status = contact_apply_status_enum.PENDING
	contactApply.Message = req.Message

	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error("Update group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// GetFriendApplyList 获取收到的好友申请列表 (我被申请为好友)
func (u *userContactService) GetFriendApplyList(userId string) ([]respond.NewContactListRespond, error) {
	// 1. 一次性查出所有待处理申请
	contactApplyList, err := u.repos.ContactApply.FindByTargetIdPending(userId)
	if err != nil {
		zap.L().Error("Find pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if len(contactApplyList) == 0 {
		return []respond.NewContactListRespond{}, nil
	}

	// 2. 【优化关键】收集所有申请人的 UUID
	userUuids := make([]string, 0, len(contactApplyList))
	for _, apply := range contactApplyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	// 3. 【优化关键】一次性批量查询所有申请人的详细资料
	userList, err := u.repos.User.FindByUuids(userUuids)
	if err != nil {
		zap.L().Error("Batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 4. 将用户信息转为 Map，方便后续快速查找
	userMap := make(map[string]model.UserInfo)
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	// 5. 组装结果
	rsp := make([]respond.NewContactListRespond, 0, len(contactApplyList))
	for _, apply := range contactApplyList {
		user, oK := userMap[apply.ApplicantId]
		if !oK {
			continue // 如果用户不存在（极端情况），跳过
		}

		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}

		rsp = append(rsp, respond.NewContactListRespond{
			ApplicantId:   user.Uuid,
			ContactName:   user.Nickname,
			ContactAvatar: user.Avatar,
			Message:       message,
		})
	}
	return rsp, nil
}

// GetGroupApplyList 获取收到的加群申请列表 (群主/管理员视角)
func (u *userContactService) GetGroupApplyList(groupId string) ([]respond.AddGroupListRespond, error) {
	// 1. 一次性获取所有待处理申请
	contactApplyList, err := u.repos.ContactApply.FindByTargetIdPending(groupId)
	if err != nil {
		zap.L().Error("Find group pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if len(contactApplyList) == 0 {
		return []respond.AddGroupListRespond{}, nil
	}

	// 2. 收集所有申请人的 UUID
	userUuids := make([]string, 0, len(contactApplyList))
	for _, apply := range contactApplyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	// 3. 批量查询用户信息
	userList, err := u.repos.User.FindByUuids(userUuids)
	if err != nil {
		zap.L().Error("Batch find users info error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 4. 用户信息转 Map
	userMap := make(map[string]model.UserInfo)
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	// 5. 组装结果
	rsp := make([]respond.AddGroupListRespond, 0, len(contactApplyList))
	for _, apply := range contactApplyList {
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			continue
		}

		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}

		rsp = append(rsp, respond.AddGroupListRespond{
			ApplicantId:   user.Uuid,
			ContactName:   user.Nickname,
			ContactAvatar: user.Avatar,
			Message:       message,
		})
	}
	return rsp, nil
}

// PassFriendApply 通过好友申请
func (u *userContactService) PassFriendApply(userId string, applicantId string) error {
	// 1. 获取申请记录
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 事务执行数据库操作
	err = u.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 校验申请人状态
		user, err := txRepos.User.FindByUuid(applicantId)
		if err != nil {
			zap.L().Error("Find user error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if user.Status == user_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
		}

		// 更新申请状态
		contactApply.Status = contact_apply_status_enum.AGREE
		if err := txRepos.ContactApply.Update(contactApply); err != nil {
			return err
		}

		// 双向建立联系人关系
		newContact := model.UserContact{
			UserId:      userId,
			ContactId:   applicantId,
			ContactType: contact_type_enum.USER,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.Create(&newContact); err != nil {
			return err
		}

		anotherContact := model.UserContact{
			UserId:      applicantId,
			ContactId:   userId,
			ContactType: contact_type_enum.USER,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.Create(&anotherContact); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步清理缓存
	myredis.SubmitCacheTask(func() {
		_ = myredis.DelKeysWithPattern("contact_user_list_" + userId)
		_ = myredis.DelKeysWithPattern("contact_user_list_" + applicantId)
	})

	return nil
}

// PassGroupApply 通过入群申请
func (u *userContactService) PassGroupApply(groupId string, applicantId string) error {
	// 1. 获取申请记录
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 事务执行数据库操作
	err = u.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 校验群组状态
		group, err := txRepos.Group.FindByUuid(groupId)
		if err != nil {
			zap.L().Error("Find group error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if group.Status == group_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该群聊已被禁用")
		}

		// 更新申请状态
		contactApply.Status = contact_apply_status_enum.AGREE
		if err := txRepos.ContactApply.Update(contactApply); err != nil {
			return err
		}

		// 建立个人与群的联系
		newContact := model.UserContact{
			UserId:      applicantId,
			ContactId:   groupId,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.Create(&newContact); err != nil {
			return err
		}

		// 添加群成员记录
		member := model.GroupMember{
			GroupUuid: groupId,
			UserUuid:  applicantId,
			Role:      1,
		}
		if err := txRepos.GroupMember.Create(&member); err != nil {
			return err
		}

		// 增加群成员计数
		if err := txRepos.Group.IncrementMemberCount(groupId); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步清理缓存
	myredis.SubmitCacheTask(func() {
		_ = myredis.DelKeysWithPattern("my_joined_group_list_" + applicantId)
		_ = myredis.DelKeysWithPattern("group_info_" + groupId)
	})

	return nil
}

// RefuseFriendApply 拒绝好友申请
func (u *userContactService) RefuseFriendApply(userId string, applicantId string) error {
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	contactApply.Status = contact_apply_status_enum.REFUSE
	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error("Update friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// RefuseGroupApply 拒绝入群申请
func (u *userContactService) RefuseGroupApply(groupId string, applicantId string) error {
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	contactApply.Status = contact_apply_status_enum.REFUSE
	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error("Update group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackContact 拉黑联系人
func (u *userContactService) BlackContact(userId string, contactId string) error {
	// 开启事务
	err := u.repos.Transaction(func(txRepos *repository.Repositories) error {
		// 1. 更新拉黑者的状态为 BLACK
		if err := txRepos.Contact.UpdateStatus(userId, contactId, contact_status_enum.BLACK); err != nil {
			zap.L().Error("Update status to BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 2. 更新被拉黑者的状态为 BE_BLACK
		if err := txRepos.Contact.UpdateStatus(contactId, userId, contact_status_enum.BE_BLACK); err != nil {
			zap.L().Error("Update status to BE_BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 3. 双方的会话进行软删除
		if err := txRepos.Session.SoftDeleteByUsers([]string{userId, contactId}); err != nil {
			zap.L().Error("Soft delete sessions error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 4. 清理缓存
	myredis.SubmitCacheTask(func() {
		_ = myredis.DelKeysWithPattern("direct_session_list_" + userId)
		_ = myredis.DelKeysWithPattern("direct_session_list_" + contactId)
		_ = myredis.DelKeysWithPattern("contact_user_list_" + userId)
		_ = myredis.DelKeysWithPattern("contact_user_list_" + contactId)
	})

	return nil
}

// CancelBlackContact 取消拉黑联系人
func (u *userContactService) CancelBlackContact(userId string, contactId string) error {
	// 1. 事务外先校验状态
	blackContact, err := u.repos.Contact.FindByUserIdAndContactId(userId, contactId)
	if err != nil {
		zap.L().Error("Find black contact error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if blackContact.Status != contact_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "未拉黑该联系人，无需解除拉黑")
	}

	beBlackContact, err := u.repos.Contact.FindByUserIdAndContactId(contactId, userId)
	if err != nil {
		zap.L().Error("Find be-black contact error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if beBlackContact.Status != contact_status_enum.BE_BLACK {
		return errorx.New(errorx.CodeInvalidParam, "该联系人未被拉黑，无需解除拉黑")
	}

	// 2. 使用事务确保双方状态更新的原子性
	err = u.repos.Transaction(func(txRepos *repository.Repositories) error {
		if err := txRepos.Contact.UpdateStatus(userId, contactId, contact_status_enum.NORMAL); err != nil {
			zap.L().Error("Update black contact status error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if err := txRepos.Contact.UpdateStatus(contactId, userId, contact_status_enum.NORMAL); err != nil {
			zap.L().Error("Update be-black contact status error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步清理缓存
	myredis.SubmitCacheTask(func() {
		_ = myredis.DelKeysWithPattern("contact_user_list_" + userId)
		_ = myredis.DelKeysWithPattern("contact_user_list_" + contactId)
	})

	return nil
}

// BlackFriendApply 拉黑好友申请
func (u *userContactService) BlackFriendApply(userId string, applicantId string) error {
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	contactApply.Status = contact_apply_status_enum.BLACK
	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error("Update friend apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackGroupApply 拉黑入群申请
func (u *userContactService) BlackGroupApply(groupId string, applicantId string) error {
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	contactApply.Status = contact_apply_status_enum.BLACK
	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error("Update group apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}
