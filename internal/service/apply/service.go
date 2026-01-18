package apply

import (
	"context"
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
	"kama_chat_server/pkg/enum/group_info/add_mode_enum"
	"kama_chat_server/pkg/enum/group_info/group_status_enum"
	"kama_chat_server/pkg/enum/user_info/user_status_enum"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/util/random"
)

// applyService 申请业务逻辑实现
type applyService struct {
	repos *mysql.Repositories
	cache myredis.AsyncCacheService
}

// NewApplyService 构造函数
func NewApplyService(repos *mysql.Repositories, cacheService myredis.AsyncCacheService) *applyService {
	return &applyService{
		repos: repos,
		cache: cacheService,
	}
}

// ApplyFriend 申请添加好友
func (u *applyService) ApplyFriend(userId string, req request.ApplyFriendRequest) error {
	if len(req.FriendId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

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

	relation, err := u.repos.Contact.FindByUserIdAndContactId(userId, req.FriendId)
	if err == nil && relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你们已经是好友")
	}

	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(userId, req.FriendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			apply = &model.Apply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: userId,
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

	if apply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑，无法发送申请")
	}

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
func (u *applyService) ApplyGroup(userId string, req request.ApplyGroupRequest) error {
	if len(req.GroupId) == 0 {
		return errorx.New(errorx.CodeInvalidParam, "群组ID不能为空")
	}

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

	relation, err := u.repos.Contact.FindByUserIdAndContactId(userId, req.GroupId)
	if err == nil && relation != nil && relation.Status == contact_status_enum.NORMAL {
		return errorx.New(errorx.CodeInvalidParam, "你已在该群中")
	}

	// 免审核群：直接入群
	if group.AddMode == add_mode_enum.DIRECT {
		err := u.repos.Transaction(func(txRepos *mysql.Repositories) error {
			member := model.GroupMember{
				GroupUuid: req.GroupId,
				UserUuid:  userId,
				Role:      1,
			}
			if err := txRepos.GroupMember.CreateGroupMember(&member); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			if err := txRepos.Group.IncrementMemberCount(req.GroupId); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			newContact := model.Contact{
				UserId:      userId,
				ContactId:   req.GroupId,
				ContactType: contact_type_enum.GROUP,
				Status:      contact_status_enum.NORMAL,
			}
			if err := txRepos.Contact.CreateContact(&newContact); err != nil {
				zap.L().Error("service error", zap.Error(err))
				return errorx.ErrServerBusy
			}

			_ = txRepos.Apply.SoftDelete(userId, req.GroupId)
			return nil
		})

		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		u.cache.SubmitTask(func() {
			_ = u.cache.DeleteByPattern(context.Background(), "group_session_list_"+req.GroupId+"*")
			_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+userId+"*")
			_ = u.cache.Delete(context.Background(), "group_info_"+req.GroupId)
		})

		return nil
	}

	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(userId, req.GroupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			apply = &model.Apply{
				Uuid:        fmt.Sprintf("A%s", random.GetNowAndLenRandomString(11)),
				ApplicantId: userId,
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

	if apply.Status == contact_apply_status_enum.BLACK {
		return errorx.New(errorx.CodeInvalidParam, "该群已将你拉黑，无法发送申请")
	}

	apply.LastApplyAt = time.Now()
	apply.Status = contact_apply_status_enum.PENDING
	apply.Message = req.Message

	if err := u.repos.Apply.Update(apply); err != nil {
		zap.L().Error("Update group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// GetFriendApplyList 获取收到的好友申请列表
func (u *applyService) GetFriendApplyList(userId string) ([]respond.NewContactListRespond, error) {
	applyList, err := u.repos.Apply.FindByTargetIdPending(userId)
	if err != nil {
		zap.L().Error("Find pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if len(applyList) == 0 {
		return []respond.NewContactListRespond{}, nil
	}

	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	userList, err := u.repos.User.FindByUuids(userUuids)
	if err != nil {
		zap.L().Error("Batch find users error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	userMap := make(map[string]model.UserInfo)
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	rsp := make([]respond.NewContactListRespond, 0, len(applyList))
	for _, apply := range applyList {
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			continue
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

// GetGroupApplyList 获取收到的加群申请列表
func (u *applyService) GetGroupApplyList(userId, groupId string) ([]respond.AddGroupListRespond, error) {
	// 权限校验: 必须是群主或管理员 (Role >= 2)
	member, err := u.repos.GroupMember.FindByGroupAndUser(groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if member.Role < 2 {
		return nil, errorx.New(errorx.CodeForbidden, "你没有查看申请列表的权限")
	}

	applyList, err := u.repos.Apply.FindByTargetIdPending(groupId)
	if err != nil {
		zap.L().Error("Find group pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if len(applyList) == 0 {
		return []respond.AddGroupListRespond{}, nil
	}

	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	userList, err := u.repos.User.FindByUuids(userUuids)
	if err != nil {
		zap.L().Error("Batch find users info error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	userMap := make(map[string]model.UserInfo)
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

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
func (u *applyService) PassFriendApply(userId string, applicantId string) error {
	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		user, err := txRepos.User.FindByUuid(applicantId)
		if err != nil {
			zap.L().Error("Find user error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if user.Status == user_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
		}

		apply.Status = contact_apply_status_enum.AGREE
		if err := txRepos.Apply.Update(apply); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		newContact := model.Contact{
			UserId:      userId,
			ContactId:   applicantId,
			ContactType: contact_type_enum.USER,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&newContact); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		anotherContact := model.Contact{
			UserId:      applicantId,
			ContactId:   userId,
			ContactType: contact_type_enum.USER,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&anotherContact); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+userId)
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:user:"+applicantId)
	})

	return nil
}

// PassGroupApply 通过入群申请
// operatorId 必须是群主或管理员
func (u *applyService) PassGroupApply(operatorId, groupId, applicantId string) error {
	// 权限校验: 检查操作者是否是群主或管理员
	member, err := u.repos.GroupMember.FindByGroupAndUser(groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if member.Role < 2 { // Role: 1=普通成员, 2=管理员, 3=群主
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

	apply, err := u.repos.Apply.FindByApplicantIdAndTargetId(applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	err = u.repos.Transaction(func(txRepos *mysql.Repositories) error {
		group, err := txRepos.Group.FindByUuid(groupId)
		if err != nil {
			zap.L().Error("Find group error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if group.Status == group_status_enum.DISABLE {
			return errorx.New(errorx.CodeInvalidParam, "该群聊已被禁用")
		}

		apply.Status = contact_apply_status_enum.AGREE
		if err := txRepos.Apply.Update(apply); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		newContact := model.Contact{
			UserId:      applicantId,
			ContactId:   groupId,
			ContactType: contact_type_enum.GROUP,
			Status:      contact_status_enum.NORMAL,
		}
		if err := txRepos.Contact.CreateContact(&newContact); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		newMember := model.GroupMember{
			GroupUuid: groupId,
			UserUuid:  applicantId,
			Role:      1,
		}
		if err := txRepos.GroupMember.CreateGroupMember(&newMember); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if err := txRepos.Group.IncrementMemberCount(groupId); err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	u.cache.SubmitTask(func() {
		_ = u.cache.DeleteByPattern(context.Background(), "contact_relation:group:"+applicantId)
		_ = u.cache.DeleteByPattern(context.Background(), "group_info_"+groupId)
	})

	return nil
}

// RefuseFriendApply 拒绝好友申请
func (u *applyService) RefuseFriendApply(userId string, applicantId string) error {
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
// operatorId 必须是群主或管理员
func (u *applyService) RefuseGroupApply(operatorId, groupId, applicantId string) error {
	// 权限校验
	member, err := u.repos.GroupMember.FindByGroupAndUser(groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if member.Role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

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

// BlackFriendApply 拉黑好友申请
func (u *applyService) BlackFriendApply(userId string, applicantId string) error {
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
// operatorId 必须是群主或管理员
func (u *applyService) BlackGroupApply(operatorId, groupId, applicantId string) error {
	// 权限校验
	member, err := u.repos.GroupMember.FindByGroupAndUser(groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Find group member error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if member.Role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

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
