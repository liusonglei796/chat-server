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
	cacheKey := "contact_user_list_" + userId

	// 1. 尝试从 Redis 获取
	rspString, err := myredis.GetKeyNilIsErr(cacheKey)
	if err == nil {
		var rsp []respond.MyUserListRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp, nil
		}
		zap.L().Error("Unmarshal user list cache error", zap.Error(err))
	}

	// 2. 检查是否是真正的 Redis 错误（非 Key 不存在）
	if err != nil && !errorx.IsNotFound(err) {
		zap.L().Error("Redis error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 3. 缓存未命中，查询数据库
	// 获取联系人 ID 列表
	contactList, err := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.USER)
	if err != nil {
		zap.L().Error("Find contact list error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 如果没有联系人，直接返回空切片并缓存（防止缓存穿透）
	if len(contactList) == 0 {
		u.setCache(cacheKey, []respond.MyUserListRespond{})
		return []respond.MyUserListRespond{}, nil
	}

	// 4. 【优化关键】提取 UUID 列表，准备批量查询
	uuids := make([]string, 0, len(contactList))
	for _, c := range contactList {
		uuids = append(uuids, c.ContactId)
	}

	// 5. 【优化关键】一次性批量查询用户信息
	// 假设你已经在 userRepo 中实现了 FindByUuids(uuids []string)
	users, err := u.repos.User.FindByUuids(uuids)
	if err != nil {
		zap.L().Error("Batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 6. 组装返回结果
	userListRsp := make([]respond.MyUserListRespond, 0, len(users))
	for _, user := range users {
		userListRsp = append(userListRsp, respond.MyUserListRespond{
			UserId:   user.Uuid,
			UserName: user.Nickname,
			Avatar:   user.Avatar,
		})
	}

	// 7. 异步或同步写入缓存
	u.setCache(cacheKey, userListRsp)

	return userListRsp, nil
}

// 辅助方法：统一设置缓存
func (u *userContactService) setCache(key string, data interface{}) {
	rspBytes, err := json.Marshal(data)
	if err != nil {
		zap.L().Error("Marshal cache error", zap.Error(err), zap.String("key", key))
		return
	}
	_ = myredis.SetKeyEx(key, string(rspBytes), time.Minute*constants.REDIS_TIMEOUT)
}

// LoadMyJoinedGroup 获取我加入的群组列表（不包含自己创建的）
func (u *userContactService) LoadMyJoinedGroup(userId string) ([]respond.LoadMyJoinedGroupRespond, error) {
	cacheKey := "my_joined_group_list_" + userId

	// 1. 尝试从缓存获取
	rspString, err := myredis.GetKeyNilIsErr(cacheKey)
	if err == nil {
		var rsp []respond.LoadMyJoinedGroupRespond
		if err := json.Unmarshal([]byte(rspString), &rsp); err == nil {
			return rsp, nil
		}
		zap.L().Error("Unmarshal group list cache error", zap.Error(err))
	}

	if err != nil && !errorx.IsNotFound(err) {
		zap.L().Error("Redis error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 3. 从数据库获取关联关系
	contactList, err := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP)
	if err != nil {
		zap.L().Error("Find contact list error", zap.Error(err), zap.String("userId", userId))
		return nil, errorx.ErrServerBusy
	}

	// 如果没有加入任何群组
	if len(contactList) == 0 {
		u.setCache(cacheKey, []respond.LoadMyJoinedGroupRespond{})
		return []respond.LoadMyJoinedGroupRespond{}, nil
	}

	// 4. 【优化关键】提取群组 UUID 列表
	groupUuids := make([]string, 0, len(contactList))
	for _, contact := range contactList {
		// 如果你的业务逻辑确实需要过滤非 'G' 开头的 ID
		if len(contact.ContactId) > 0 && contact.ContactId[0] == 'G' {
			groupUuids = append(groupUuids, contact.ContactId)
		}
	}

	// 5. 【优化关键】批量查询群组信息
	// 假设 groupRepo 实现了 FindByUuids
	groups, err := u.repos.Group.FindByUuids(groupUuids)
	if err != nil {
		zap.L().Error("Batch find groups error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 6. 构造返回结果并过滤掉自己创建的群
	groupListRsp := make([]respond.LoadMyJoinedGroupRespond, 0, len(groups))
	for _, group := range groups {
		// 过滤逻辑：只添加非本人创建的群
		if group.OwnerId != userId {
			groupListRsp = append(groupListRsp, respond.LoadMyJoinedGroupRespond{
				GroupId:   group.Uuid,
				GroupName: group.Name,
				Avatar:    group.Avatar,
			})
		}
	}

	// 7. 写入缓存
	u.setCache(cacheKey, groupListRsp)

	return groupListRsp, nil
}

// GetContactInfo 获取联系人信息
// 如果你点的是人：显示的是那个人的昵称、头像。
// 如果你点的是群：显示的是群的名字、群头像。
func (u *userContactService) GetContactInfo(contactId string) (respond.GetContactInfoRespond, error) {
	// 1. 安全检查，防止空字符串导致 panic
	if len(contactId) == 0 {
		return respond.GetContactInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "ID不能为空")
	}

	// 1. 尝试从缓存获取 (根据 ID 前缀区分类型)
	var cacheKey string
	if contactId[0] == 'G' {
		cacheKey = "group_info_" + contactId
	} else {
		cacheKey = "user_info_" + contactId
	}

	cachedStr, err := myredis.GetKey(cacheKey)
	if err == nil && cachedStr != "" {
		// 注意：这里的缓存结构可能与 GetContactInfoRespond 不完全一致
		// GetUserInfoRespond 和 GetGroupInfoRespond 是基础信息
		// 我们需要根据前缀分别处理缓存的反序列化
		if contactId[0] == 'G' {
			var groupRsp respond.GetGroupInfoRespond
			if err := json.Unmarshal([]byte(cachedStr), &groupRsp); err == nil {
				return respond.GetContactInfoRespond{
					ContactId:        groupRsp.Uuid,
					ContactName:      groupRsp.Name,
					ContactAvatar:    groupRsp.Avatar,
					ContactNotice:    groupRsp.Notice,
					ContactAddMode:   groupRsp.AddMode,
					ContactMemberCnt: groupRsp.MemberCnt,
					ContactOwnerId:   groupRsp.OwnerId,
				}, nil
			}
		} else {
			var userRsp respond.GetUserInfoRespond
			if err := json.Unmarshal([]byte(cachedStr), &userRsp); err == nil {
				return respond.GetContactInfoRespond{
					ContactId:        userRsp.Uuid,
					ContactName:      userRsp.Nickname,
					ContactAvatar:    userRsp.Avatar,
					ContactBirthday:  userRsp.Birthday,
					ContactEmail:     userRsp.Email,
					ContactPhone:     userRsp.Telephone,
					ContactGender:    userRsp.Gender,
					ContactSignature: userRsp.Signature,
				}, nil
			}
		}
		// 如果反序列化失败，记录错误并继续从数据库查询
		zap.L().Error("Unmarshal contact info cache error", zap.Error(err), zap.String("cacheKey", cacheKey))
	}

	// 2. 缓存未命中，处理群组 ID
	if contactId[0] == 'G' {
		group, err := u.repos.Group.FindByUuid(contactId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return respond.GetContactInfoRespond{}, errorx.New(errorx.CodeNotFound, "该群聊不存在")
			}
			zap.L().Error("Find group error", zap.Error(err), zap.String("contactId", contactId))
			return respond.GetContactInfoRespond{}, errorx.ErrServerBusy
		}

		if group.Status == group_status_enum.DISABLE {
			return respond.GetContactInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "该群聊处于禁用状态")
		}

		rsp := respond.GetContactInfoRespond{
			ContactId:        group.Uuid,
			ContactName:      group.Name,
			ContactAvatar:    group.Avatar,
			ContactNotice:    group.Notice,
			ContactAddMode:   group.AddMode,
			ContactMemberCnt: group.MemberCnt,
			ContactOwnerId:   group.OwnerId,
		}

		// 回写缓存 (使用 GroupInfoRespond 的格式一致性)
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

	// 3. 处理用户 ID
	user, err := u.repos.User.FindByUuid(contactId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return respond.GetContactInfoRespond{}, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		zap.L().Error("Find user error", zap.Error(err), zap.String("contactId", contactId))
		return respond.GetContactInfoRespond{}, errorx.ErrServerBusy
	}

	// 检查用户状态
	if user.Status == user_status_enum.DISABLE {
		return respond.GetContactInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "该用户处于禁用状态")
	}

	rsp := respond.GetContactInfoRespond{
		ContactId:        user.Uuid,
		ContactName:      user.Nickname,
		ContactAvatar:    user.Avatar,
		ContactBirthday:  user.Birthday,
		ContactEmail:     user.Email,
		ContactPhone:     user.Telephone,
		ContactGender:    user.Gender,
		ContactSignature: user.Signature,
	}

	// 回写缓存 (使用 UserInfoRespond 的格式一致性)
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
	go func() {
		_ = myredis.DelKeysWithPattern("contact_user_list_" + userId)
		_ = myredis.DelKeysWithPattern("direct_session_list_" + userId)
	}()

	return nil
}

// ApplyContact 申请添加联系人
func (u *userContactService) ApplyContact(req request.ApplyContactRequest) error {
	// 1. 安全检查
	if len(req.ContactId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "目标ID不能为空")
	}

	// 2. 校验目标是否存在且有效
	var contactType int8
	if req.ContactId[0] == 'U' {
		contactType = contact_type_enum.USER
		user, err := u.repos.User.FindByUuid(req.ContactId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeUserNotExist, "该用户不存在")
			}
			return errorx.ErrServerBusy
		}
		if user.Status == user_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
		}
	} else if req.ContactId[0] == 'G' {
		contactType = contact_type_enum.GROUP
		group, err := u.repos.Group.FindByUuid(req.ContactId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "该群聊不存在")
			}
			return errorx.ErrServerBusy
		}
		if group.Status == group_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该群聊已被禁用")
		}
	} else {
		return errorx.New(errorx.CodeInvalidParam, "非法ID格式")
	}

	// 3. 【新增优化】检查是否已经是好友/已在群中，防止重复操作
	relation, err := u.repos.Contact.FindByUserIdAndContactId(req.UserId, req.ContactId)
	if err == nil && relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你们已经是好友/已在群中")
	}

	// 4. 获取或创建申请记录 (检查是否已经给对方发送过申请)
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(req.UserId, req.ContactId)
	if err != nil {
		if errorx.IsNotFound(err) {
			// 如果没查到，说明是第一次申请，创建一个新的
			contactApply = &model.ContactApply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: req.UserId,    // 申请发起人 (我)
				TargetId:    req.ContactId, // 申请目标 (对方或群组)
				ContactType: contactType,   // 申请类型 (0:个人, 1:群组)
				Status:      contact_apply_status_enum.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
			}
			if err := u.repos.ContactApply.Create(contactApply); err != nil {
				zap.L().Error("Create apply error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			return nil
		}
		zap.L().Error("Find apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 5. 现存记录的状态校验（黑名单等）
	if contactApply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑，无法发送申请")
	}

	// 6. 更新旧记录：重置状态为 PENDING，更新时间和留言
	contactApply.LastApplyAt = time.Now()
	contactApply.Status = contact_apply_status_enum.PENDING
	contactApply.Message = req.Message // 允许用户更新申请理由

	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error("Update apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// GetNewContactList 获取收到的好友申请列表 (我被申请为好友)
func (u *userContactService) GetNewContactList(userId string) ([]respond.NewContactListRespond, error) {
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

// GetAddGroupList 获取收到的加群申请列表 (群主/管理员视角)
func (u *userContactService) GetAddGroupList(groupId string) ([]respond.AddGroupListRespond, error) {
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

// PassContactApply 通过联系人申请
// targetId: 被申请的目标（用户ID或群组ID）
// applicantId: 申请人的用户ID
func (u *userContactService) PassContactApply(targetId string, applicantId string) error {
	// 1. 获取申请记录
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, targetId)
	if err != nil {
		zap.L().Error("Find contact apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 事务执行数据库操作
	err = u.repos.Transaction(func(txRepos *repository.Repositories) error {
		if targetId[0] == 'U' {
			// 处理个人好友申请
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
				UserId:      targetId,
				ContactId:   applicantId,
				ContactType: contact_type_enum.USER,
				Status:      contact_status_enum.NORMAL,
			}
			if err := txRepos.Contact.Create(&newContact); err != nil {
				return err
			}

			anotherContact := model.UserContact{
				UserId:      applicantId,
				ContactId:   targetId,
				ContactType: contact_type_enum.USER,
				Status:      contact_status_enum.NORMAL,
			}
			if err := txRepos.Contact.Create(&anotherContact); err != nil {
				return err
			}
			return nil
		}

		// 处理入群申请
		group, err := txRepos.Group.FindByUuid(targetId)
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
			ContactId:   targetId,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.Create(&newContact); err != nil {
			return err
		}

		// 添加群成员记录
		member := model.GroupMember{
			GroupUuid: targetId,
			UserUuid:  applicantId,
			Role:      1,
		}
		if err := txRepos.GroupMember.Create(&member); err != nil {
			return err
		}

		// 增加群成员计数
		if err := txRepos.Group.IncrementMemberCount(targetId); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步清理缓存 (不阻塞主流程，且只有成功后才清理)
	go func() {
		if targetId[0] == 'U' {
			// 好友申请通过：清理双方的联系人列表缓存
			_ = myredis.DelKeysWithPattern("contact_user_list_" + targetId)
			_ = myredis.DelKeysWithPattern("contact_user_list_" + applicantId)
		} else {
			// 入群申请通过：清理申请人的群组列表缓存和群信息缓存
			_ = myredis.DelKeysWithPattern("my_joined_group_list_" + applicantId)
			_ = myredis.DelKeysWithPattern("group_info_" + targetId)
		}
	}()

	return nil
}

// RefuseContactApply 拒绝联系人申请
// targetId: 被申请的目标（用户ID或群组ID）
// applicantId: 申请人的用户ID
func (u *userContactService) RefuseContactApply(targetId string, applicantId string) error {
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, targetId)
	if err != nil {
		zap.L().Error(err.Error())
		return errorx.ErrServerBusy
	}
	contactApply.Status = contact_apply_status_enum.REFUSE
	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error(err.Error())
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
			// 会话删除失败可以考虑是否回滚，这里选择回滚以确保一致性
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 4. 清理缓存
	go func() {
		_ = myredis.DelKeysWithPattern("direct_session_list_" + userId)
		_ = myredis.DelKeysWithPattern("direct_session_list_" + contactId)
		_ = myredis.DelKeysWithPattern("contact_user_list_" + userId)
		_ = myredis.DelKeysWithPattern("contact_user_list_" + contactId)
	}()

	return nil
}

// CancelBlackContact 取消拉黑联系人
// SQL 逻辑: 事务内更新双方状态 BLACK -> NORMAL, BE_BLACK -> NORMAL
func (u *userContactService) CancelBlackContact(userId string, contactId string) error {
	// 1. 事务外先校验状态，避免不必要的事务开销
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
	go func() {
		_ = myredis.DelKeysWithPattern("contact_user_list_" + userId)
		_ = myredis.DelKeysWithPattern("contact_user_list_" + contactId)
	}()

	return nil
}

// BlackApply 拉黑申请
// targetId: 被申请的目标（用户ID或群组ID）
// applicantId: 申请人的用户ID
// SQL 逻辑: UPDATE contact_applies SET status = 'BLACK' WHERE user_id = ? AND contacted_id = ?
func (u *userContactService) BlackApply(targetId string, applicantId string) error {
	contactApply, err := u.repos.ContactApply.FindByApplicantIdAndTargetId(applicantId, targetId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find contact apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	contactApply.Status = contact_apply_status_enum.BLACK
	if err := u.repos.ContactApply.Update(contactApply); err != nil {
		zap.L().Error("Update contact apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}
