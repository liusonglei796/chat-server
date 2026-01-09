package contact

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
	"kama_chat_server/internal/model"
	"kama_chat_server/pkg/enum/contact/contact_status_enum"
	"kama_chat_server/pkg/enum/contact/contact_type_enum"
	"kama_chat_server/pkg/enum/contact_apply/contact_apply_status_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/random"
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
func (u *contactService) GetFriendInfo(friendId string) (respond.GetFriendInfoRespond, error) {
	// 1. 安全检查
	if len(friendId) == 0 {
		return respond.GetFriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
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
func (u *contactService) GetGroupDetail(groupId string) (respond.GetGroupDetailRespond, error) {
	// 1. 安全检查
	if len(groupId) == 0 {
		return respond.GetGroupDetailRespond{}, errorx.New(errorx.CodeInvalidParam, "群聊ID不能为空")
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
	// 使用事务确保操作原子性
	err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
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
		_ = txRepos.Apply.SoftDelete(userId, contactId)

		return nil
	})

	if err != nil {
		return err
	}

	// 4. 异步清理"我的"缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.RemoveFromSet(context.Background(), "contact_relation:user:"+userId, contactId)
		_ = u.cache.DeleteByPattern(context.Background(), "direct_session_list_"+userId)
	})

	return nil
}

// ApplyFriend 申请添加好友
func (u *contactService) ApplyFriend(req request.ApplyFriendRequest) error {
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
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(req.UserId, req.FriendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			// 第一次申请，创建新记录
			apply = &model.Apply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: req.UserId,
				TargetId:    req.FriendId,
				ContactType: contact_type_enum.USER,
				Status:      contact_apply_status_enum.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
			}
			if err := u.repos.Apply.CreateApply(apply); err != nil {
				zap.L().Error("Create friend apply error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			return nil
		}
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 5. 黑名单校验
	if apply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑，无法发送申请")
	}

	// 6. 更新旧记录
	apply.LastApplyAt = time.Now()
	apply.Status = contact_apply_status_enum.PENDING
	apply.Message = req.Message

	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// ApplyGroup 申请加入群组
func (u *contactService) ApplyGroup(req request.ApplyGroupRequest) error {
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
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(req.UserId, req.GroupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			// 第一次申请，创建新记录
			apply = &model.Apply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: req.UserId,
				TargetId:    req.GroupId,
				ContactType: contact_type_enum.GROUP,
				Status:      contact_apply_status_enum.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
			}
			if err := u.repos.Apply.CreateApply(apply); err != nil {
				zap.L().Error("Create group apply error", zap.Error(err))
				return errorx.ErrServerBusy
			}
			return nil
		}
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 5. 黑名单校验
	if apply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "该群已将你拉黑，无法发送申请")
	}

	// 6. 更新旧记录
	apply.LastApplyAt = time.Now()
	apply.Status = contact_apply_status_enum.PENDING
	apply.Message = req.Message

	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// GetFriendApplyList 获取收到的好友申请列表 (我被申请为好友)
func (u *contactService) GetFriendApplyList(userId string) ([]respond.NewContactListRespond, error) {
	// 1. 一次性查出所有待处理申请
	applyList, err := u.repos.Apply.FindByTargetIdPending(userId)
	if err != nil {
		zap.L().Error("Find pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if len(applyList) == 0 {
		return []respond.NewContactListRespond{}, nil
	}

	// 2. 【优化关键】收集所有申请人的 UUID
	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
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
	rsp := make([]respond.NewContactListRespond, 0, len(applyList))
	for _, apply := range applyList {
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
func (u *contactService) GetGroupApplyList(groupId string) ([]respond.AddGroupListRespond, error) {
	// 1. 一次性获取所有待处理申请
	applyList, err := u.repos.Apply.FindByTargetIdPending(groupId)
	if err != nil {
		zap.L().Error("Find group pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if len(applyList) == 0 {
		return []respond.AddGroupListRespond{}, nil
	}

	// 2. 收集所有申请人的 UUID
	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
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
	rsp := make([]respond.AddGroupListRespond, 0, len(applyList))
	for _, apply := range applyList {
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
func (u *contactService) PassFriendApply(userId string, applicantId string) error {
	// 1. 获取申请记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 事务执行数据库操作
	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
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
		apply.Status = contact_apply_status_enum.AGREE
		if err := txRepos.Apply.Update(apply); err != nil {
			return err
		}

		// 双向建立联系人关系
		newContact := model.Contact{
			UserId:      userId,
			ContactId:   applicantId,
			ContactType: contact_type_enum.USER,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&newContact); err != nil {
			return err
		}

		anotherContact := model.Contact{
			UserId:      applicantId,
			ContactId:   userId,
			ContactType: contact_type_enum.USER,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&anotherContact); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 3. 异步清理缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+applicantId)
	})

	return nil
}

// PassGroupApply 通过入群申请
func (u *contactService) PassGroupApply(groupId string, applicantId string) error {
	// 1. 获取申请记录
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// 2. 事务执行数据库操作
	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
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
		apply.Status = contact_apply_status_enum.AGREE
		if err := txRepos.Apply.Update(apply); err != nil {
			return err
		}

		// 建立个人与群的联系
		newContact := model.Contact{
			UserId:      applicantId,
			ContactId:   groupId,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&newContact); err != nil {
			return err
		}

		// 添加群成员记录
		member := model.GroupMember{
			GroupUuid: groupId,
			UserUuid:  applicantId,
			Role:      1,
		}
		if err := txRepos.GroupMember.CreateGroupMember(&member); err != nil {
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
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+applicantId)
		_ = u.cache.DeleteByPattern(context.Background(), "group_info_"+groupId)
	})

	return nil
}

// RefuseFriendApply 拒绝好友申请
func (u *contactService) RefuseFriendApply(userId string, applicantId string) error {
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	apply.Status = contact_apply_status_enum.REFUSE
	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// RefuseGroupApply 拒绝入群申请
func (u *contactService) RefuseGroupApply(groupId string, applicantId string) error {
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	apply.Status = contact_apply_status_enum.REFUSE
	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackContact 拉黑联系人
func (u *contactService) BlackContact(userId string, contactId string) error {
	// 开启事务
	err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
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
		return err
	}

	// 3. 异步清理缓存
	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+contactId)
	})

	return nil
}

// BlackFriendApply 拉黑好友申请
func (u *contactService) BlackFriendApply(userId string, applicantId string) error {
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	apply.Status = contact_apply_status_enum.BLACK
	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update friend apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackGroupApply 拉黑入群申请
func (u *contactService) BlackGroupApply(groupId string, applicantId string) error {
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	apply.Status = contact_apply_status_enum.BLACK
	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update group apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	return nil
}
