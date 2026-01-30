package contact

import (
	"context"
	"encoding/json"

	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/respond/contact"
	groupdto "kama_chat_server/internal/dto/respond/group"
	userdto "kama_chat_server/internal/dto/respond/user"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/contact/contact_type_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
)

// contactService 联系人业务逻辑实现
// 通过构造函数注入 Repository 和 Cache 依赖，遵循依赖倒置原则
type contactService struct {
	repos *mysql.Repositories
	cache myredis.AsyncCacheService
}

// NewContactService 构造函数，注入所有依赖
func NewContactService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *contactService {
	return &contactService{
		repos: repos,
		cache: cacheService,
	}
}

// GetUserList 获取指定用户的“好友（联系人）的用户信息列表”。
func (u *contactService) GetUserList(userId string, page, pageSize int) ([]userdto.MyUserListRespond, int64, error) {
	// 参数校验
	if page < 1 { page = 1 }
	if pageSize < 1 || pageSize > 100 { pageSize = 20 }

	// 从数据库分页查询联系人
	contactList, total, err := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.USER, page, pageSize)
	if err != nil {
		zap.L().Error("Find contact list error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	if len(contactList) == 0 {
		return []userdto.MyUserListRespond{}, total, nil
	}

	// 批量获取好友信息（混合模式：优先 Redis，回退 DB）
	userListRsp := make([]userdto.MyUserListRespond, 0, len(contactList))
	for _, contact := range contactList {
		cacheKey := "user_info_" + contact.ContactId
		val, err := u.cache.Get(context.Background(), cacheKey)
		if err == nil && val != "" {
			var userInfo userdto.GetUserInfoRespond
			if err := json.Unmarshal([]byte(val), &userInfo); err == nil {
				userListRsp = append(userListRsp, userdto.MyUserListRespond{
					UserId:   userInfo.Uuid,
					UserName: userInfo.Nickname,
					Avatar:   userInfo.Avatar,
				})
				continue
			}
		}
		
		// 缓存未命中，查数据库
		userInfo, dbErr := u.repos.User.FindByUuid(contact.ContactId)
		if dbErr != nil {
			zap.L().Error("Find user by uuid error", zap.Error(dbErr))
			continue
		}
		
		// 组装响应
		userListRsp = append(userListRsp, userdto.MyUserListRespond{
			UserId:   userInfo.Uuid,
			UserName: userInfo.Nickname,
			Avatar:   userInfo.Avatar,
		})
		
		// 回写缓存
		if userInfoStr, err := json.Marshal(userInfo); err == nil {
			_ = u.cache.Set(context.Background(), cacheKey, string(userInfoStr), 300) // 5分钟过期
		}
	}

	return userListRsp, total, nil
}

// GetGroupList 获取用户的群组列表（所有加入的群组）
// 与 GetUserList 类似，采用 Cache-Aside 模式
func (u *contactService) GetGroupList(userId string, page, pageSize int) ([]groupdto.MyGroupListRespond, int64, error) {
	// 参数校验
	if page < 1 { page = 1 }
	if pageSize < 1 || pageSize > 100 { pageSize = 20 }

	// 从数据库分页查询群组
	contactList, total, err := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP, page, pageSize)
	if err != nil {
		zap.L().Error("Find group list error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	if len(contactList) == 0 {
		return []groupdto.MyGroupListRespond{}, total, nil
	}

	// 批量获取群组信息
	groupListRsp := make([]groupdto.MyGroupListRespond, 0, len(contactList))
	for _, contact := range contactList {
		groupInfo, err := u.repos.Group.FindByUuid(contact.ContactId)
		if err != nil {
			zap.L().Error("Find group by uuid error", zap.Error(err))
			continue
		}

		groupListRsp = append(groupListRsp, groupdto.MyGroupListRespond{
			GroupId:   groupInfo.Uuid,
			GroupName: groupInfo.Name,
			Avatar:    groupInfo.Avatar,
		})
	}

	return groupListRsp, total, nil
}

// GetFriendInfo 获取好友详情 (userId 必须与 friendId 是好友关系)
func (u *contactService) GetFriendInfo(userId, friendId string) (contact.FriendInfoRespond, error) {
	// 1. 安全检查和权限校验
	if len(friendId) == 0 {
		return contact.FriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	// 校验是否是好友关系
	isFriend, err := u.repos.Contact.IsFriend(userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return contact.FriendInfoRespond{}, errorx.ErrServerBusy
	}
	if !isFriend {
		return contact.FriendInfoRespond{}, errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	// 2. 尝试从缓存获取
	cacheKey := "user_info_" + friendId
	cachedStr, err := u.cache.Get(context.Background(), cacheKey)
	if err == nil && cachedStr != "" {
		var userRsp userdto.GetUserInfoRespond
		if err := json.Unmarshal([]byte(cachedStr), &userRsp); err == nil {
			return contact.FriendInfoRespond{
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
			return contact.FriendInfoRespond{}, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		zap.L().Error("Find user error", zap.Error(err), zap.String("friendId", friendId))
		return contact.FriendInfoRespond{}, errorx.ErrServerBusy
	}

	// 4. 检查用户状态
	if user.Status == user_status_enum.DISABLE {
		return contact.FriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "该用户处于禁用状态")
	}

	rsp := contact.FriendInfoRespond{
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
	userRsp := userdto.GetUserInfoRespond{
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
		_ = u.cache.Set(context.Background(), cacheKey, string(data), time.Hour)
	}

	return rsp, nil
}

// GetGroupDetail 获取群聊详情 (userId 必须是群成员)
func (u *contactService) GetGroupDetail(userId, groupId string) (contact.GroupDetailRespond, error) {
	// 1. 安全检查和权限校验
	if len(groupId) == 0 {
		return contact.GroupDetailRespond{}, errorx.New(errorx.CodeInvalidParam, "群聊ID不能为空")
	}

	// 校验是否是群成员
	_, err := u.repos.GroupMember.FindByGroupAndUser(groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return contact.GroupDetailRespond{}, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Check group membership error", zap.Error(err))
		return contact.GroupDetailRespond{}, errorx.ErrServerBusy
	}

	// 2. 尝试从缓存获取
	cacheKey := "group_info_" + groupId
	cachedStr, err := u.cache.Get(context.Background(), cacheKey)
	if err == nil && cachedStr != "" {
		var groupRsp groupdto.GetGroupInfoRespond
		if err := json.Unmarshal([]byte(cachedStr), &groupRsp); err == nil {
			return contact.GroupDetailRespond{
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
			return contact.GroupDetailRespond{}, errorx.New(errorx.CodeNotFound, "该群聊不存在")
		}
		zap.L().Error("Find group error", zap.Error(err), zap.String("groupId", groupId))
		return contact.GroupDetailRespond{}, errorx.ErrServerBusy
	}

	// 4. 检查群组状态
	if group.Status == group_status_enum.DISABLE {
		return contact.GroupDetailRespond{}, errorx.New(errorx.CodeInvalidParam, "该群聊处于禁用状态")
	}

	rsp := contact.GroupDetailRespond{
		GroupId:     group.Uuid,
		GroupName:   group.Name,
		GroupAvatar: group.Avatar,
		GroupNotice: group.Notice,
		MemberCnt:   group.MemberCnt,
		OwnerId:     group.OwnerId,
		AddMode:     group.AddMode,
	}

	// 5. 回写缓存
	groupRsp := groupdto.GetGroupInfoRespond{
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
		_ = u.cache.Set(context.Background(), cacheKey, string(data), time.Hour)
	}

	return rsp, nil
}

// DeleteContact 删除联系人
func (u *contactService) DeleteContact(userId, contactId string) error {
	// 校验是否是好友关系
	isFriend, err := u.repos.Contact.IsFriend(userId, contactId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !isFriend {
		return errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}
	// 使用事务确保操作原子性
	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 1. 仅从“我的”联系人列表中移除对方 (单向操作)
		if err := txRepos.Contact.SoftDelete(userId, contactId, contact_type_enum.USER); err != nil {
			zap.L().Error("Delete contact relation error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 2. 仅清理“我的”视角下的会话 (Session)
		// 历史需求变更: 仅删除联系人关系，保留会话历史(Read-Only)
		// 因此不再执行 Session.SoftDelete

		// 3. 清理“我的”视角下的申请记录 (可选，通常为了防止再次申请时逻辑混淆)
		_ = txRepos.Apply.SoftDelete(userId, contactId)

		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 4. 异步清理"我的"缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.RemoveFromSet(context.Background(), "contact_relation:user:"+userId, contactId)
		_ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+userId)
	})

	return nil
}

// BlackContact 拉黑联系人
// userId: 操作者（发起拉黑的用户）
// contactId: 被拉黑的联系人
func (u *contactService) BlackContact(userId string, contactId string) error {
	// 1. 参数校验：不能拉黑自己
	if userId == contactId {
		return errorx.New(errorx.CodeInvalidParam, "不能拉黑自己")
	}

	// 2. 开启事务（将好友关系校验也放入事务内，防止并发情况下的竞态条件）
	err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 2.1 校验是否是好友（在事务内校验，保证一致性）
		myContact, err := txRepos.Contact.FindByUserIdAndContactId(userId, contactId, contact_type_enum.USER)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeForbidden, "你们还不是好友，无法拉黑")
			}
			zap.L().Error("Find contact error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if myContact.Status != contact_status_enum.NORMAL {
			return errorx.New(errorx.CodeInvalidParam, "当前状态不允许拉黑")
		}

		// 2.2 更新拉黑者的状态为 BLACK
		if err := txRepos.Contact.UpdateStatus(userId, contactId, contact_type_enum.USER, contact_status_enum.BLACK); err != nil {
			zap.L().Error("Update status to BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 2.3 更新被拉黑者的状态为 BE_BLACK
		if err := txRepos.Contact.UpdateStatus(contactId, userId, contact_type_enum.USER, contact_status_enum.BE_BLACK); err != nil {
			zap.L().Error("Update status to BE_BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		return nil
	})

	if err != nil {
		// 如果是业务错误（如 CodeForbidden），直接返回，不要包装
		if e, ok := err.(*errorx.CodeError); ok {
			return e
		}
		zap.L().Error("BlackContact transaction error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 3. 异步清理缓存
	// 使用 RemoveFromSet 精准删除，而非 DeleteByPattern 全量删除
	u.cache.SubmitTask(func() {
		// 从双方的好友列表缓存中移除对方
		_ = u.cache.RemoveFromSet(context.Background(), "contact_relation:user:"+userId, contactId)
		_ = u.cache.RemoveFromSet(context.Background(), "contact_relation:user:"+contactId, userId)
		// 会话列表缓存暂时保留（历史可查看），如需删除可取消注释
		// _ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+userId)
		// _ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+contactId)
	})

	return nil
}

// CancelBlackContact 取消拉黑联系人
// userId: 操作者（发起解除拉黑的用户，即之前拉黑对方的人）
// contactId: 被解除拉黑的联系人
func (u *contactService) CancelBlackContact(userId string, contactId string) error {
	// 1. 参数校验：不能解除拉黑自己
	if userId == contactId {
		return errorx.New(errorx.CodeInvalidParam, "参数错误")
	}

	// 2. 开启事务（将状态校验也放入事务内，防止并发情况下的竞态条件）
	err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		// 2.1 校验拉黑者的状态
		myContact, err := txRepos.Contact.FindByUserIdAndContactId(userId, contactId, contact_type_enum.USER)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "联系人关系不存在")
			}
			zap.L().Error("Find black contact error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if myContact.Status != contact_status_enum.BLACK {
			return errorx.New(errorx.CodeInvalidParam, "未拉黑该联系人，无需解除拉黑")
		}

		// 2.2 校验被拉黑者的状态
		theirContact, err := txRepos.Contact.FindByUserIdAndContactId(contactId, userId, contact_type_enum.USER)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "对方联系人关系不存在")
			}
			zap.L().Error("Find be-black contact error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if theirContact.Status != contact_status_enum.BE_BLACK {
			return errorx.New(errorx.CodeInvalidParam, "数据状态异常，请联系管理员")
		}

		// 2.3 更新双方状态为 NORMAL
		if err := txRepos.Contact.UpdateStatus(userId, contactId, contact_type_enum.USER, contact_status_enum.NORMAL); err != nil {
			zap.L().Error("Update black contact status error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if err := txRepos.Contact.UpdateStatus(contactId, userId, contact_type_enum.USER, contact_status_enum.NORMAL); err != nil {
			zap.L().Error("Update be-black contact status error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		return nil
	})

	if err != nil {
		// 如果是业务错误，直接返回，不要包装
		if e, ok := err.(*errorx.CodeError); ok {
			return e
		}
		zap.L().Error("CancelBlackContact transaction error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 3. 异步恢复缓存
	// 使用 AddToSet 将双方重新加入对方的好友列表缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.AddToSet(context.Background(), "contact_relation:user:"+userId, contactId)
		_ = u.cache.AddToSet(context.Background(), "contact_relation:user:"+contactId, userId)
	})

	return nil
}
