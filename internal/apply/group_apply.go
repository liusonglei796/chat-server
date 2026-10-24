package apply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/internal/infra/snowflake"
	"kama_chat_server/pkg/enum/apply/apply_status"
	"kama_chat_server/pkg/enum/apply/apply_type"
	"kama_chat_server/pkg/enum/group/add_mode"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/event"
	"kama_chat_server/pkg/idempotent"
)

// ApplyGroup 申请加入群组
func (u *ApplyService) ApplyGroup(ctx context.Context, userId string, req ApplyGroupRequest) error {
	action := func(execCtx context.Context) (struct{}, error) {
		if len(req.GroupId) == 0 {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "群组ID不能为空")
		}

		groupRsp, err := grpc_client.GetGroupDetail(execCtx, userId, req.GroupId)
		if err != nil {
			return struct{}{}, err
		}

		isMember, err := grpc_client.CheckGroupMember(execCtx, req.GroupId, userId)
		if err == nil && isMember {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "你已在该群中")
		}

		apply, err := u.store.FindByApplicantIdAndTargetId(execCtx, userId, req.GroupId)
		if err != nil {
			if errorx.IsNotFound(err) {
				apply = &Apply{
					Uuid:        fmt.Sprintf("A%s", snowflake.GenerateIDString()),
					ApplicantId: userId,
					TargetId:    req.GroupId,
					ContactType: apply_type.GROUP,
					Status:      apply_status.PENDING,
					Message:     req.Message,
					LastApplyAt: time.Now(),
					Version:     1,
				}

				if int8(groupRsp.AddMode) == add_mode.DIRECT {
					apply.Status = apply_status.AGREE
					err = database.ExecTx(execCtx, u.db, func(txCtx context.Context) error {
						if err := u.store.CreateApply(txCtx, apply); err != nil {
							return err
						}
						payload, _ := json.Marshal(event.GroupApplyPassedEvent{
							GroupId: req.GroupId,
							UserId:  userId,
						})
						return u.recordOutboxEvent(txCtx, req.GroupId, event.EventGroupApplyPassed, payload)
					})
					if err != nil {
						zap.L().Error("Direct join group error", zap.Error(err))
						return struct{}{}, errorx.ErrServerBusy
					}
					return struct{}{}, nil
				}

				if err := u.store.CreateApply(execCtx, apply); err != nil {
					if dberr.IsDuplicateKey(err) {
						if existing, getErr := u.store.FindByApplicantIdAndTargetId(execCtx, userId, req.GroupId); getErr == nil {
							if existing.Status == apply_status.BLACK {
								return struct{}{}, errorx.New(errorx.CodeInvalidParam, "你已被该群拉黑，无法申请加入")
							}
							existing.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
							existing.LastApplyAt = time.Now()
							existing.Status = apply_status.PENDING
							existing.Message = req.Message
							existing.Version += 1
							return struct{}{}, u.store.Update(execCtx, existing)
						}
					}
					zap.L().Error("Create group apply error", zap.Error(err))
					return struct{}{}, errorx.ErrServerBusy
				}
				return struct{}{}, nil
			}
			zap.L().Error("Find group apply error", zap.Error(err))
			return struct{}{}, errorx.ErrServerBusy
		}

		if apply.Status == apply_status.BLACK {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "你已被该群拉黑，无法申请加入")
		}

		if int8(groupRsp.AddMode) == add_mode.DIRECT {
			apply.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
			apply.LastApplyAt = time.Now()
			apply.Status = apply_status.AGREE
			apply.Message = req.Message
			apply.Version += 1

			err = database.ExecTx(execCtx, u.db, func(txCtx context.Context) error {
				if err := u.store.Update(txCtx, apply); err != nil {
					return err
				}
				payload, _ := json.Marshal(event.GroupApplyPassedEvent{
					GroupId: req.GroupId,
					UserId:  userId,
				})
				return u.recordOutboxEvent(txCtx, req.GroupId, event.EventGroupApplyPassed, payload)
			})
			if err != nil {
				zap.L().Error("Direct join group error", zap.Error(err))
				return struct{}{}, errorx.ErrServerBusy
			}
			return struct{}{}, nil
		}

		oldVersion := apply.Version
		apply.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
		apply.LastApplyAt = time.Now()
		apply.Status = apply_status.PENDING
		apply.Message = req.Message
		apply.Version += 1

		ok, err := u.store.UpdateWithVersion(execCtx, apply, oldVersion)
		if err != nil {
			zap.L().Error("Update group apply error", zap.Error(err))
			return struct{}{}, errorx.ErrServerBusy
		}
		if !ok {
			return struct{}{}, errorx.New(errorx.CodeConflict, "申请状态发生变化，请刷新重试")
		}

		return struct{}{}, nil
	}

	if u.guard != nil && req.ClientToken != "" {
		key := "apply:group:" + req.ClientToken
		_, _, err := idempotent.ExecuteJSON(ctx, u.guard, key, 5*time.Second, 24*time.Hour, action)
		return err
	}

	_, err := action(ctx)
	return err
}

// GetGroupApplyList 获取收到的加群申请列表
func (u *ApplyService) GetGroupApplyList(ctx context.Context, userId, groupId string, page, pageSize int) (*PagedGroupApplyListRespond, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	role, err := grpc_client.GetGroupMemberRole(ctx, groupId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil, errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Get group member role error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}
	if role < 2 {
		return nil, errorx.New(errorx.CodeForbidden, "你没有查看加群申请的权限")
	}

	applyList, total, err := u.store.FindByTargetIdPendingPaged(ctx, groupId, page, pageSize)
	if err != nil {
		zap.L().Error("Find pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	if total == 0 || len(applyList) == 0 {
		return &PagedGroupApplyListRespond{
			Total: total,
			List:  []GroupApplyListRespond{},
		}, nil
	}

	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	userList, err := grpc_client.BatchGetPublicUserInfo(ctx, userUuids)
	if err != nil {
		zap.L().Error("batch get applicants via grpc error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	userMap := make(map[string]*userpb.PublicUserInfo, len(userList))
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	rsp := make([]GroupApplyListRespond, 0, len(applyList))
	for _, apply := range applyList {
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			continue
		}

		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}

		rsp = append(rsp, GroupApplyListRespond{
			ApplicantId:     user.Uuid,
			ApplicantName:   user.Nickname,
			ApplicantAvatar: user.Avatar,
			Message:         message,
		})
	}

	return &PagedGroupApplyListRespond{
		Total: total,
		List:  rsp,
	}, nil
}

// PassGroupApply 通过入群申请
func (u *ApplyService) PassGroupApply(ctx context.Context, operatorId, groupId, applicantId string) error {
	role, err := grpc_client.GetGroupMemberRole(ctx, groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Get group member role error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

	apply, err := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if apply.Status == apply_status.AGREE {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	isMember, err := grpc_client.CheckGroupMember(ctx, groupId, applicantId)
	if err == nil && isMember {
		return errorx.New(errorx.CodeInvalidParam, "该用户已在该群中")
	}

	var alreadyAgreed bool
	err = database.ExecTx(ctx, u.db, func(txCtx context.Context) error {
		ok, err := u.store.UpdateStatusIfPending(txCtx, apply.Uuid, apply_status.AGREE, apply.Version)
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok {
			cur, queryErr := u.store.FindByApplicantIdAndTargetId(txCtx, applicantId, groupId)
			if queryErr != nil || cur == nil {
				return errorx.New(errorx.CodeNotFound, "申请记录不存在")
			}
			if cur.TargetId != groupId {
				return errorx.New(errorx.CodeForbidden, "目标群组不匹配")
			}
			if cur.Status == apply_status.REFUSE {
				return errorx.New(errorx.CodeInvalidParam, "该申请已被拒绝")
			}
			if cur.Status == apply_status.BLACK {
				return errorx.New(errorx.CodeInvalidParam, "该申请已被拉黑")
			}
			if cur.Status == apply_status.AGREE {
				alreadyAgreed = true
				return nil
			}
			return errorx.ErrServerBusy
		}

		payload, _ := json.Marshal(event.GroupApplyPassedEvent{
			GroupId: groupId,
			UserId:  applicantId,
		})
		return u.recordOutboxEvent(txCtx, groupId, event.EventGroupApplyPassed, payload)
	})

	if err != nil {
		var codeErr *errorx.CodeError
		if errors.As(err, &codeErr) {
			return err
		}
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if alreadyAgreed {
		return nil
	}

	return nil
}

// RefuseGroupApply 拒绝入群申请
func (u *ApplyService) RefuseGroupApply(ctx context.Context, operatorId, groupId, applicantId string) error {
	role, err := grpc_client.GetGroupMemberRole(ctx, groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Get group member role error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

	apply, err := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if apply.Status == apply_status.REFUSE {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	ok, err := u.store.UpdateStatusIfPending(ctx, apply.Uuid, apply_status.REFUSE, apply.Version)
	if err != nil {
		zap.L().Error("Update group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !ok {
		cur, queryErr := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
		if queryErr != nil || cur == nil {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		if cur.TargetId != groupId {
			return errorx.New(errorx.CodeForbidden, "目标群组不匹配")
		}
		if cur.Status == apply_status.AGREE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被同意，无法拒绝")
		}
		if cur.Status == apply_status.BLACK {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被拉黑")
		}
		if cur.Status == apply_status.REFUSE {
			return nil
		}
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackGroupApply 拉黑入群申请
func (u *ApplyService) BlackGroupApply(ctx context.Context, operatorId, groupId, applicantId string) error {
	role, err := grpc_client.GetGroupMemberRole(ctx, groupId, operatorId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeForbidden, "你不是该群成员")
		}
		zap.L().Error("Get group member role error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if role < 2 {
		return errorx.New(errorx.CodeForbidden, "你没有审批权限")
	}

	apply, err := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	if apply.Status == apply_status.BLACK {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	ok, err := u.store.UpdateStatusIfPending(ctx, apply.Uuid, apply_status.BLACK, apply.Version)
	if err != nil {
		zap.L().Error("Update group apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !ok {
		cur, queryErr := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
		if queryErr != nil || cur == nil {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		if cur.TargetId != groupId {
			return errorx.New(errorx.CodeForbidden, "目标群组不匹配")
		}
		if cur.Status == apply_status.AGREE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被同意，无法拉黑")
		}
		if cur.Status == apply_status.REFUSE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被拒绝，无法拉黑")
		}
		if cur.Status == apply_status.BLACK {
			return nil
		}
		return errorx.ErrServerBusy
	}
	return nil
}
