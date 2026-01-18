package contact

import (
	"context"
	"encoding/json"

	"time"

	"go.uber.org/zap"

	"kama_chat_server/internal/dao/mysql"
	myredis "kama_chat_server/internal/dao/redis"
	"kama_chat_server/internal/dto/respond"
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
func (u *contactService) GetUserList(userId string) ([]respond.MyUserListRespond, error) {
	// 优化：使用 Redis Set 存储好友 ID (contact_relation:user:<uid>)
	// 这可以避免存储巨大的 JSON 列表，并确保与 UserInfo 缓存的数据一致性。
	cacheKey := "contact_relation:user:" + userId

	// 1. 尝试从缓存获取成员 ID（通过注入的 cache 接口）
	memberIds, err := u.cache.GetSetMembers(context.Background(), cacheKey)
	if err != nil || len(memberIds) == 0 {
		// 2. 缓存未击中或为空：从数据库获取
		contactList, dbErr := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.USER)
		if dbErr != nil {
			zap.L().Error("Find contact list error", zap.Error(dbErr))
			return nil, errorx.ErrServerBusy
		}

		// 重新填充 memberIds
		memberIds = make([]string, 0, len(contactList))
		for _, c := range contactList {
			memberIds = append(memberIds, c.ContactId)
		}

		// 回写到 Redis（如果不为空）
		if len(memberIds) > 0 {
			membersArgs := make([]interface{}, len(memberIds))
			for i, v := range memberIds {
				membersArgs[i] = v
			}
			_ = u.cache.AddToSet(context.Background(), cacheKey, membersArgs...)
		}
	}

	if len(memberIds) == 0 {
		return []respond.MyUserListRespond{}, nil
	}

	// 3. 批量获取用户信息（数据源或用户缓存）
	// 理想情况下，我们应该首先从 Redis MGET "user_info:<id>"，然后回退到数据库。
	// 为了简单和一致，我们使用 Repo 的 FindByUuids，它通常查询数据库。
	// 如果性能至关重要，Repos 应该处理实体的缓存。
	users, err := u.repos.User.FindByUuids(memberIds)
	if err != nil {
		zap.L().Error("Batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 4. 组装响应
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

// GetJoinedGroupsExcludedOwn 获取我加入的群组列表（不包含自己创建的）
// 从 LoadMyJoinedGroup 重命名以清晰表达逻辑。
func (u *contactService) GetJoinedGroupsExcludedOwn(userId string) ([]respond.LoadMyJoinedGroupRespond, error) {
	// 优化：为群组 ID 使用 Redis Set
	cacheKey := "contact_relation:group:" + userId

	// 1. 尝试从缓存获取群组 ID
	groupUuids, err := u.cache.GetSetMembers(context.Background(), cacheKey)
	if err != nil || len(groupUuids) == 0 {
		// 2. 缓存未击中：从数据库获取
		contactList, dbErr := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP)
		if dbErr != nil {
			zap.L().Error("Find joined groups error", zap.Error(dbErr))
			return nil, errorx.ErrServerBusy
		}

		// 过滤 ID（以防万一，必须防止非 G 前缀）
		groupUuids = make([]string, 0, len(contactList))
		for _, contact := range contactList {
			if len(contact.ContactId) > 0 && contact.ContactId[0] == 'G' {
				groupUuids = append(groupUuids, contact.ContactId)
			}
		}

		// 回写到缓存
		if len(groupUuids) > 0 {
			args := make([]interface{}, len(groupUuids))
			for i, v := range groupUuids {
				args[i] = v
			}
			_ = u.cache.AddToSet(context.Background(), cacheKey, args...)
		}
	}

	if len(groupUuids) == 0 {
		return []respond.LoadMyJoinedGroupRespond{}, nil
	}

	// 3. 批量获取群组信息
	groups, err := u.repos.Group.FindByUuids(groupUuids)
	if err != nil {
		zap.L().Error("Batch find groups error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 4. 组装响应（在此过滤 OwnerId，以确保安全并严格遵守“排除自己”的逻辑）
	// 虽然理论上 Redis Set 应该只包含有效的加入群组，
	// 但加强过滤逻辑可确保一致性。
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
// GetFriendInfo 获取好友详情 (userId 必须与 friendId 是好友关系)
func (u *contactService) GetFriendInfo(userId, friendId string) (respond.GetFriendInfoRespond, error) {
	// 1. 安全检查和权限校验
	if len(friendId) == 0 {
		return respond.GetFriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	// 校验是否是好友关系
	isFriend, err := u.repos.Contact.IsFriend(userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return respond.GetFriendInfoRespond{}, errorx.ErrServerBusy
	}
	if !isFriend {
		return respond.GetFriendInfoRespond{}, errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	// 2. 尝试从缓存获取
	cacheKey := "user_info_" + friendId
	cachedStr, err := u.cache.Get(context.Background(), cacheKey)
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
		_ = u.cache.Set(context.Background(), cacheKey, string(data), time.Hour)
	}

	return rsp, nil
}

// GetGroupDetail 获取群聊详情
// GetGroupDetail 获取群聊详情 (userId 必须是群成员)
func (u *contactService) GetGroupDetail(userId, groupId string) (respond.GetGroupDetailRespond, error) {
	// 1. 安全检查和权限校验
	if len(groupId) == 0 {
		return respond.GetGroupDetailRespond{}, errorx.New(errorx.CodeInvalidParam, "群聊ID不能为空")
	}

	// 校验是否是群成员
	_, err := u.repos.GroupMember.FindByGroupAndUser(groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return respond.GetGroupDetailRespond{}, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Check group membership error", zap.Error(err))
		return respond.GetGroupDetailRespond{}, errorx.ErrServerBusy
	}

	// 2. 尝试从缓存获取
	cacheKey := "group_info_" + groupId
	cachedStr, err := u.cache.Get(context.Background(), cacheKey)
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
		if err := txRepos.Contact.SoftDelete(userId, contactId); err != nil {
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
func (u *contactService) BlackContact(userId string, contactId string) error {
	// 校验是否存在关系(不限制必须是好友，但至少要有 contact 记录，或者业务只允许拉黑好友？)
	// 根据用户描述"没有校验是不是在好友"，倾向于限制只能拉黑好友。
	// 但通常IM也可以拉黑陌生人。这里严格照应用户需求：校验是否是好友。
	isFriend, err := u.repos.Contact.IsFriend(userId, contactId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !isFriend {
		return errorx.New(errorx.CodeForbidden, "你们还不是好友，无法拉黑")
	}
	// 开启事务
	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
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
		// 3. 双方的会话进行软删除 (已废弃：保留历史)
		// if err := txRepos.Session.SoftDeleteByUsers([]string{userId, contactId}); err != nil {
		// 	zap.L().Error("Soft delete sessions error", zap.Error(err))
		// 	return errorx.ErrServerBusy
		// }
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 4. 清理缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+contactId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+contactId)
	})

	return nil
}

// CancelBlackContact 取消拉黑联系人
func (u *contactService) CancelBlackContact(userId string, contactId string) error {
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
	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
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
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 3. 异步清理缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+contactId)
	})

	return nil
}
