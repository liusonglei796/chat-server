package apply

import (
	"context"
	"errors"
	"fmt"
	applypb "kama_chat_server/api/gen/apply"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/internal/infra/idempotency"
	"kama_chat_server/internal/infra/snowflake"
	"kama_chat_server/pkg/enum/apply/apply_status"
	"kama_chat_server/pkg/enum/apply/apply_type"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// ApplyFriend 申请添加好友
// userId: 发起申请的用户ID
// req: 包含目标好友ID和申请信息的请求对象
func (u *ApplyService) ApplyFriend(ctx context.Context, userId string, req ApplyFriendRequest) error {
	// Step 1.1: 声明好友申请执行体闭包
	action := func(execCtx context.Context) (struct{}, error) {
		// 1. 参数校验
		if len(req.FriendId) == 0 {
			return struct{}{}, applypb.ErrorInvalidParam("好友ID不能为空")
		}

		if userId == req.FriendId {
			return struct{}{}, applypb.ErrorCannotApplySelf("不能添加自己为好友")
		}

		// 2. 检查目标用户是否存在与状态
		userStatus, err := grpc_client.GetUserStatus(execCtx, req.FriendId)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return struct{}{}, applypb.ErrorUserNotFound("该用户不存在")
			}
			return struct{}{}, applypb.ErrorServerBusy("服务繁忙")
		}

		if userStatus == user_status.DISABLE {
			return struct{}{}, applypb.ErrorInvalidParam("该用户已被禁用")
		}

		// 3. 检查是否已经是好友或处于黑名单
		statusVal, err := grpc_client.CheckFriendshipStatus(execCtx, userId, req.FriendId)
		if err == nil {
			if statusVal == 1 { // 1=正常好友
				return struct{}{}, applypb.ErrorAlreadyFriends("你们已经是好友")
			}
			if statusVal == 3 { // 3=被对方拉黑
				return struct{}{}, applypb.ErrorBlockedByUser("对方已将你拉黑，无法发送申请")
			}
		}

		// 微信同款“静默免验证加回”：检查目标方 (req.FriendId) 是否仍保留着我 (userId)
		reverseStatus, err := grpc_client.CheckFriendshipStatus(execCtx, req.FriendId, userId)
		if err == nil && reverseStatus == 4 {
			// reverseStatus == 4 表示对方通讯录中仍完好保留着我，但我此前单向删除了对方
			// 此时直接静默建立单向好友关系，无需走审批流，对方零感知
			if estErr := grpc_client.EstablishFriendship(execCtx, userId, req.FriendId); estErr != nil {
				return struct{}{}, estErr
			}
			return struct{}{}, nil
		}

		// 4-6. 申请记录的读取与写入在同一事务内，由数据库唯一键与 CAS 保证并发正确性
		return struct{}{}, database.ExecTx(execCtx, u.db, func(txCtx context.Context) error {
			return u.upsertFriendApply(txCtx, userId, req)
		})
	}

	// Step 1.2: 优先采用 MySQL 持久化幂等表进行事务级防重与回放
	if req.ClientToken != "" && u.idempStore != nil {
		type applyFriendResp struct {
			Status string `json:"status"`
		}
		_, _, err := idempotency.ExecuteJSON(ctx, u.idempStore, userId, "ApplyFriend", req.ClientToken, func(execCtx context.Context) (*applyFriendResp, error) {
			if _, aErr := action(execCtx); aErr != nil {
				return nil, aErr
			}
			return &applyFriendResp{Status: "ok"}, nil
		})
		return err
	} else if u.guard != nil && req.ClientToken != "" {
		key := "apply:friend:" + req.ClientToken
		_, _, err := idempotent.ExecuteJSON(ctx, u.guard, key, 5*time.Second, 24*time.Hour, action)
		return err
	}

	// Step 1.3: 无防重凭证时的直接执行兜底
	_, err := action(ctx)
	return err
}

// upsertFriendApply 在事务内创建或更新好友申请记录
func (u *ApplyService) upsertFriendApply(ctx context.Context, userId string, req ApplyFriendRequest) error {
	apply, err := u.store.FindByApplicantIdAndTargetId(ctx, userId, req.FriendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			apply = &Apply{
				Uuid:        fmt.Sprintf("A%s", snowflake.GenerateIDString()),
				ApplicantId: userId,
				TargetId:    req.FriendId,
				ContactType: apply_type.USER,
				Status:      apply_status.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
				Version:     1,
			}
			if err := u.store.CreateApply(ctx, apply); err != nil {
				if dberr.IsDuplicateKey(err) {
					if existing, getErr := u.store.FindByApplicantIdAndTargetId(ctx, userId, req.FriendId); getErr == nil {
						if existing.Status == apply_status.BLACK {
							return applypb.ErrorBlockedByUser("对方已将你拉黑，无法发送申请")
						}
						existing.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
						existing.LastApplyAt = time.Now()
						existing.Status = apply_status.PENDING
						existing.Message = req.Message
						existing.Version++
						return u.store.Update(ctx, existing)
					}
				}
				zap.L().Error("Create friend apply error", zap.Error(err))
				return applypb.ErrorServerBusy("服务繁忙")
			}
			return nil
		}
		zap.L().Error("Find friend apply error", zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}

	// 黑名单检查
	if apply.Status == apply_status.BLACK {
		return applypb.ErrorBlockedByUser("对方已将你拉黑，无法发送申请")
	}

	// 更新旧申请记录（重置状态并刷新 Uuid 与 Version，防止前代审批时序倒退穿透）
	oldVersion := apply.Version
	apply.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
	apply.LastApplyAt = time.Now()
	apply.Status = apply_status.PENDING
	apply.Message = req.Message
	apply.Version++

	ok, err := u.store.UpdateWithVersion(ctx, apply, oldVersion)
	if err != nil {
		zap.L().Error("Update friend apply error", zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}
	if !ok {
		return applypb.ErrorApplyConflict("申请状态发生变化，请刷新重试")
	}
	return nil
}

// GetFriendApplyList 获取好友申请列表
func (u *ApplyService) GetFriendApplyList(ctx context.Context, userId string, page, pageSize int) (*PagedFriendApplyListRespond, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	applyList, total, err := u.store.FindByTargetIdPendingPaged(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find pending applies error", zap.Error(err))
		return nil, applypb.ErrorServerBusy("服务繁忙")
	}

	if total == 0 || len(applyList) == 0 {
		return &PagedFriendApplyListRespond{
			Total: total,
			List:  []FriendApplyListRespond{},
		}, nil
	}

	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	userList, err := grpc_client.BatchGetPublicUserInfo(ctx, userUuids)
	if err != nil {
		zap.L().Error("batch get applicants via grpc error", zap.Error(err))
		return nil, applypb.ErrorServerBusy("服务繁忙")
	}

	userMap := make(map[string]*userpb.PublicUserInfo, len(userList))
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	rsp := make([]FriendApplyListRespond, 0, len(applyList))
	for _, apply := range applyList {
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			continue
		}

		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}

		rsp = append(rsp, FriendApplyListRespond{
			ApplicantId:     user.Uuid,
			ApplicantName:   user.Nickname,
			ApplicantAvatar: user.Avatar,
			Message:         message,
		})
	}

	return &PagedFriendApplyListRespond{
		Total: total,
		List:  rsp,
	}, nil
}

// PassFriendApply 通过好友申请
func (u *ApplyService) PassFriendApply(ctx context.Context, userId string, applicantId string) error {
	apply, err := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}

	if apply.Status == apply_status.AGREE {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return applypb.ErrorApplyAlreadyProcessed("该申请已被处理，无法重复操作")
	}

	applicantStatus, err := grpc_client.GetUserStatus(ctx, applicantId)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return applypb.ErrorUserNotFound("该用户不存在")
		}
		zap.L().Error("Get applicant status error", zap.String("applicantId", applicantId), zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}
	if applicantStatus == user_status.DISABLE {
		return applypb.ErrorInvalidParam("该用户已被禁用")
	}

	var alreadyAgreed bool
	err = database.ExecTx(ctx, u.db, func(txCtx context.Context) error {
		ok, err := u.store.UpdateStatusIfPending(txCtx, apply.Uuid, apply_status.AGREE, apply.Version)
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return applypb.ErrorServerBusy("服务繁忙")
		}
		if !ok {
			cur, queryErr := u.store.FindByApplicantIdAndTargetId(txCtx, applicantId, userId)
			if queryErr != nil || cur == nil {
				return applypb.ErrorApplyNotFound("申请记录不存在")
			}
			if cur.TargetId != userId {
				return applypb.ErrorNoPermission("无权处理该好友申请")
			}
			if cur.Status == apply_status.REFUSE {
				return applypb.ErrorApplyAlreadyProcessed("该申请已被拒绝")
			}
			if cur.Status == apply_status.BLACK {
				return applypb.ErrorApplyAlreadyProcessed("该申请已被拉黑")
			}
			if cur.Status == apply_status.AGREE {
				alreadyAgreed = true
				return nil
			}
			return applypb.ErrorServerBusy("服务繁忙")
		}

		return u.recordFriendApplyPassedEvent(txCtx, userId, applicantId)
	})

	if err != nil {
		var codeErr *errorx.CodeError
		if errors.As(err, &codeErr) {
			return err
		}
		zap.L().Error("service error", zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}

	if alreadyAgreed {
		return nil
	}

	return nil
}

// RefuseFriendApply 拒绝好友申请
func (u *ApplyService) RefuseFriendApply(ctx context.Context, userId string, applicantId string) error {
	apply, err := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}

	if apply.Status == apply_status.REFUSE {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return applypb.ErrorApplyAlreadyProcessed("该申请已被处理，无法重复操作")
	}

	ok, err := u.store.UpdateStatusIfPending(ctx, apply.Uuid, apply_status.REFUSE, apply.Version)
	if err != nil {
		zap.L().Error("Update friend apply error", zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}
	if !ok {
		cur, queryErr := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, userId)
		if queryErr != nil || cur == nil {
			return applypb.ErrorApplyNotFound("申请记录不存在")
		}
		if cur.TargetId != userId {
			return applypb.ErrorNoPermission("无权处理该好友申请")
		}
		if cur.Status == apply_status.AGREE {
			return applypb.ErrorApplyAlreadyProcessed("该申请已被同意，无法拒绝")
		}
		if cur.Status == apply_status.BLACK {
			return applypb.ErrorApplyAlreadyProcessed("该申请已被拉黑")
		}
		if cur.Status == apply_status.REFUSE {
			return nil
		}
		return applypb.ErrorServerBusy("服务繁忙")
	}
	return nil
}

// BlackFriendApply 拉黑好友申请
func (u *ApplyService) BlackFriendApply(ctx context.Context, userId string, applicantId string) error {
	apply, err := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return applypb.ErrorApplyNotFound("申请记录不存在")
		}
		zap.L().Error("Find friend apply error", zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}

	if apply.Status == apply_status.BLACK {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return applypb.ErrorApplyAlreadyProcessed("该申请已被处理，无法重复操作")
	}

	ok, err := u.store.UpdateStatusIfPending(ctx, apply.Uuid, apply_status.BLACK, apply.Version)
	if err != nil {
		zap.L().Error("Update friend apply status error", zap.Error(err))
		return applypb.ErrorServerBusy("服务繁忙")
	}
	if !ok {
		cur, queryErr := u.store.FindByApplicantIdAndTargetId(ctx, applicantId, userId)
		if queryErr != nil || cur == nil {
			return applypb.ErrorApplyNotFound("申请记录不存在")
		}
		if cur.TargetId != userId {
			return applypb.ErrorNoPermission("无权处理该好友申请")
		}
		if cur.Status == apply_status.AGREE {
			return applypb.ErrorApplyAlreadyProcessed("该申请已被同意，无法拉黑")
		}
		if cur.Status == apply_status.REFUSE {
			return applypb.ErrorApplyAlreadyProcessed("该申请已被拒绝，无法拉黑")
		}
		if cur.Status == apply_status.BLACK {
			return nil
		}
		return applypb.ErrorServerBusy("服务繁忙")
	}
	return nil
}
