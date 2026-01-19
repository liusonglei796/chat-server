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

	// 3. 批量获取好友信息（混合模式：优先 Redis，回退 DB）
	userListRsp := make([]respond.MyUserListRespond, 0, len(memberIds))
	missingIds := make([]string, 0)

	for _, memberId := range memberIds {
		cacheKey := "user_info_" + memberId
		val, err := u.cache.Get(context.Background(), cacheKey)
		if err == nil && val != "" {
			var userInfo respond.GetUserInfoRespond
			if err := json.Unmarshal([]byte(val), &userInfo); err == nil {
				userListRsp = append(userListRsp, respond.MyUserListRespond{
					UserId:   userInfo.Uuid,
					UserName: userInfo.Nickname,
					Avatar:   userInfo.Avatar,
				})
				continue
			}
		}
		//只要以前从缓存里没拿到有效数据（不管是没找到、Redis挂了、还是网络抖动），我们都统统视为“确实没拿到”，然后去数据库里查。
		//Cache Miss (正常未命中)：
		/*Redis 里确实没有这个 Key，返回的 err 可能是 redis.Nil（取决于驱动实现）或者 val 为空。这时候必须去数据库查，否则用户列表就少了一个人。
		  Cache Error (Redis 故障)：
		  如果 Redis 突然挂了、连接超时、或者网络有问题，err 会不为空。
		  在这种情况下，我们不能直接报错返回，因为数据在 MySQL 里是完好的。
		  我们选择降级处理：忽略 Redis 的错误，把这个 ID 加入 missingIds，让它走数据库查询的路径。这样用户感受不到 Redis 挂了，只是接口稍微慢了一点点，但功能依然正常。
		  Data Corruption (数据损坏)：
		  即便是 json.Unmarshal 失败了（代码中虽然是在 if 块内但原理类似），这部分逻辑也会“跳过”成功处理的步骤，走到最后。
		  这给了系统一个自我修复的机会：从 DB 查到正确数据后，后续的异步逻辑会把正确的数据再次写入 Redis，覆盖掉坏数据。*/
		missingIds = append(missingIds, memberId)
	}

	// 4. 对未命中的数据查询数据库并回写缓存
	if len(missingIds) > 0 {
		users, err := u.repos.User.FindByUuids(missingIds)
		if err != nil {
			zap.L().Error("Batch find users error", zap.Error(err))
			return nil, errorx.ErrServerBusy
		}

		for _, user := range users {
			userListRsp = append(userListRsp, respond.MyUserListRespond{
				UserId:   user.Uuid,
				UserName: user.Nickname,
				Avatar:   user.Avatar,
			})

			// 异步回写缓存
			cacheUser := user // Copy for closure
			u.cache.SubmitTask(func() {
				info := respond.GetUserInfoRespond{
					Uuid:      cacheUser.Uuid,
					Telephone: cacheUser.Telephone,
					Nickname:  cacheUser.Nickname,
					Avatar:    cacheUser.Avatar,
					Birthday:  cacheUser.Birthday,
					Email:     cacheUser.Email,
					Gender:    cacheUser.Gender,
					Signature: cacheUser.Signature,
					CreatedAt: cacheUser.CreatedAt.Format("2006-01-02 15:04:05"),
					IsAdmin:   cacheUser.IsAdmin,
					Status:    cacheUser.Status,
				}
				if data, err := json.Marshal(info); err == nil {
					_ = u.cache.Set(context.Background(), "user_info_"+cacheUser.Uuid, string(data), time.Hour*24)
				}
			})
		}
	}

	return userListRsp, nil
}

// GetGroupList 获取用户的群组列表（所有加入的群组）
// 与 GetUserList 类似，采用 Cache-Aside 模式
func (u *contactService) GetGroupList(userId string) ([]respond.MyGroupListRespond, error) {
	cacheKey := "contact_relation:group:" + userId

	// 1. 尝试从缓存获取群组 ID
	groupIds, err := u.cache.GetSetMembers(context.Background(), cacheKey)
	if err != nil || len(groupIds) == 0 {
		// 2. 缓存未击中：从数据库获取
		contactList, dbErr := u.repos.Contact.FindByUserIdAndType(userId, contact_type_enum.GROUP)
		if dbErr != nil {
			zap.L().Error("Find group contact list error", zap.Error(dbErr))
			return nil, errorx.ErrServerBusy
		}

		// 重新填充 groupIds
		groupIds = make([]string, 0, len(contactList))
		for _, c := range contactList {
			if len(c.ContactId) > 0 && c.ContactId[0] == 'G' {
				groupIds = append(groupIds, c.ContactId)
			}
		}

		// 回写到 Redis（如果不为空）
		if len(groupIds) > 0 {
			groupArgs := make([]interface{}, len(groupIds))
			for i, v := range groupIds {
				groupArgs[i] = v
			}
			_ = u.cache.AddToSet(context.Background(), cacheKey, groupArgs...)
		}
	}

	if len(groupIds) == 0 {
		return []respond.MyGroupListRespond{}, nil
	}

	// 3. 批量获取群组信息（混合模式：优先 Redis，回退 DB）
	groupListRsp := make([]respond.MyGroupListRespond, 0, len(groupIds))
	missingIds := make([]string, 0)

	for _, groupId := range groupIds {
		infoCacheKey := "group_info_" + groupId
		val, err := u.cache.Get(context.Background(), infoCacheKey)
		if err == nil && val != "" {
			var groupInfo respond.GetGroupInfoRespond
			if err := json.Unmarshal([]byte(val), &groupInfo); err == nil {
				groupListRsp = append(groupListRsp, respond.MyGroupListRespond{
					GroupId:   groupInfo.Uuid,
					GroupName: groupInfo.Name,
					Avatar:    groupInfo.Avatar,
				})
				continue
			}
		}
		missingIds = append(missingIds, groupId)
	}

	// 4. 对未命中的数据查询数据库并回写缓存
	if len(missingIds) > 0 {
		groups, err := u.repos.Group.FindByUuids(missingIds)
		if err != nil {
			zap.L().Error("Batch find groups error", zap.Error(err))
			return nil, errorx.ErrServerBusy
		}

		for _, group := range groups {
			groupListRsp = append(groupListRsp, respond.MyGroupListRespond{
				GroupId:   group.Uuid,
				GroupName: group.Name,
				Avatar:    group.Avatar,
			})

			// 异步回写缓存
			cacheGroup := group // Copy for closure
			u.cache.SubmitTask(func() {
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
					_ = u.cache.Set(context.Background(), "group_info_"+cacheGroup.Uuid, string(data), time.Hour*24)
				}
			})
		}
	}

	return groupListRsp, nil
}

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
		myContact, err := txRepos.Contact.FindByUserIdAndContactId(userId, contactId)
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
		if err := txRepos.Contact.UpdateStatus(userId, contactId, contact_status_enum.BLACK); err != nil {
			zap.L().Error("Update status to BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 2.3 更新被拉黑者的状态为 BE_BLACK
		if err := txRepos.Contact.UpdateStatus(contactId, userId, contact_status_enum.BE_BLACK); err != nil {
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
		myContact, err := txRepos.Contact.FindByUserIdAndContactId(userId, contactId)
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
		theirContact, err := txRepos.Contact.FindByUserIdAndContactId(contactId, userId)
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
