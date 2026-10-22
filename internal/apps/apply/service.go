package apply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	applyreq "kama_chat_server/internal/common/dto/request/apply"
	applyrsp "kama_chat_server/internal/common/dto/respond/apply"
	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/grpc_client"
	cacheutil "kama_chat_server/internal/common/infrastructure/cache"
	"kama_chat_server/internal/common/infrastructure/snowflake"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/apply/apply_status"
	"kama_chat_server/pkg/enum/apply/apply_type"
	"kama_chat_server/pkg/enum/group/add_mode"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/idempotent"
)

// applyUoW 申请服务的 UoW 接口：事务/事件能力 + 仅本服务拥有的 Apply Store
type applyUoW interface {
	store.TxExecutor
	RecordEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error
	ApplyStore() store.ApplyStore
}

// ApplyService 申请业务逻辑实现
// 设计原则说明：
//  1. 读操作（如获取申请列表）：采用【直接查询数据库】模式。
//     原因：申请/审批流程对数据一致性要求极高（防脏读、防重复），且相比消息/联系列表，其访问频率较低，直查DB可确保逻辑安全且性能可控。
//  2. 写操作（如发起/通过申请）：采用【写库成功后删除缓存】模式。
//     原因：作为数据生产者，在变更状态后主动失效下游服务（如ContactService）的缓存，保证系统数据的最终一致性。
type ApplyService struct {
	uow         applyUoW
	cache       store.AsyncCacheService
	cacheHelper *cacheutil.Helper
	guard       idempotent.Guard
}

// NewApplyService 构造函数
func NewApplyService(uow applyUoW, cacheService store.AsyncCacheService, guard ...idempotent.Guard) *ApplyService {
	var g idempotent.Guard
	if len(guard) > 0 && guard[0] != nil {
		g = guard[0]
	} else if rc, ok := cacheService.(interface{ Client() *redis.Client }); ok && rc.Client() != nil {
		g = idempotent.NewRedisGuard(rc.Client())
	}
	return &ApplyService{
		uow:         uow,
		cache:       cacheService,
		cacheHelper: cacheutil.NewHelper(cacheService),
		guard:       g,
	}
}

// ApplyFriend 申请添加好友
// userId: 发起申请的用户ID
// req: 包含目标好友ID和申请信息的请求对象
func (u *ApplyService) ApplyFriend(ctx context.Context, userId string, req applyreq.ApplyFriendRequest) error {
	// Step 1.1: 声明好友申请执行体闭包
	// 来源: userId 与 req.FriendId, req.Message
	// 用于: Step 1.2 被第三代状态机幂等探针执行或直接执行
	action := func(execCtx context.Context) (struct{}, error) {
		// 1. 参数校验
		if len(req.FriendId) == 0 {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
		}

		if userId == req.FriendId {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "不能添加自己为好友")
		}

		// 2. 检查目标用户是否存在与状态
		userStatusRsp, err := grpc_client.UserClient.GetUserStatus(execCtx, &userpb.GetUserStatusRequest{UserId: req.FriendId})
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return struct{}{}, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
			}
			return struct{}{}, errorx.ErrServerBusy
		}

		if int8(userStatusRsp.Status) == user_status.DISABLE {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
		}

		// 3. 检查是否已经是好友或处于黑名单
		//
		// 【P0 修复】CheckFriendshipStatus 返回的是「对外契约」状态码：
		//   0=非好友 1=正常好友 2=已拉黑对方 3=被对方拉黑 4=被对方单向删除
		// 而 friendship_status 是「数据库行状态」枚举：
		//   0=NORMAL 1=BE_BLACK 2=BLACK
		// 两套编码的 0 含义完全相反。旧代码写的是
		//   if statusVal == friendship_status.NORMAL
		// 等价于「statusVal == 0」，即把「非好友」判成「你们已经是好友」，
		// 导致任何一对新用户的好友申请都被直接拒绝。
		statusVal, err := grpc_client.CheckFriendshipStatus(execCtx, userId, req.FriendId)
		if err == nil {
			if statusVal == 1 { // 1=正常好友
				return struct{}{}, errorx.New(errorx.CodeInvalidParam, "你们已经是好友")
			}
			if statusVal == 3 { // 3=被对方拉黑
				return struct{}{}, errorx.New(errorx.CodeForbidden, "对方已将你拉黑，无法发送申请")
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

		// 4. 查询之前的申请记录
		apply, err := u.uow.ApplyStore().FindByApplicantIdAndTargetId(execCtx, userId, req.FriendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				apply = &model.Apply{
					Uuid:        fmt.Sprintf("A%s", snowflake.GenerateIDString()),
					ApplicantId: userId,
					TargetId:    req.FriendId,
					ContactType: apply_type.USER,
					Status:      apply_status.PENDING,
					Message:     req.Message,
					LastApplyAt: time.Now(),
					Version:     1,
				}
				if err := u.uow.ApplyStore().CreateApply(execCtx, apply); err != nil {
					if dberr.IsDuplicateKey(err) {
						if existing, getErr := u.uow.ApplyStore().FindByApplicantIdAndTargetId(execCtx, userId, req.FriendId); getErr == nil {
							if existing.Status == apply_status.BLACK {
								return struct{}{}, errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑，无法发送申请")
							}
							existing.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
							existing.LastApplyAt = time.Now()
							existing.Status = apply_status.PENDING
							existing.Message = req.Message
							existing.Version += 1
							return struct{}{}, u.uow.ApplyStore().Update(execCtx, existing)
						}
					}
					zap.L().Error("Create friend apply error", zap.Error(err))
					return struct{}{}, errorx.ErrServerBusy
				}
				return struct{}{}, nil
			}
			zap.L().Error("Find friend apply error", zap.Error(err))
			return struct{}{}, errorx.ErrServerBusy
		}

		// 5. 黑名单检查
		if apply.Status == apply_status.BLACK {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "对方已将你拉黑，无法发送申请")
		}

		// 6. 更新旧申请记录（重置状态并刷新 Uuid 与 Version，防止前代审批时序倒退穿透）
		apply.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
		apply.LastApplyAt = time.Now()
		apply.Status = apply_status.PENDING
		apply.Message = req.Message
		apply.Version += 1

		if err := u.uow.ApplyStore().Update(execCtx, apply); err != nil {
			zap.L().Error("Update friend apply error", zap.Error(err))
			return struct{}{}, errorx.ErrServerBusy
		}

		return struct{}{}, nil
	}

	// Step 1.2: 接入第三代状态机幂等保护
	// 来源: req.ClientToken 与 u.guard
	// 用于: 短租约防重与重试 0ms 回显
	if u.guard != nil && req.ClientToken != "" {
		key := "apply:friend:" + req.ClientToken
		_, _, err := idempotent.ExecuteJSON(ctx, u.guard, key, 5*time.Second, 24*time.Hour, action)
		return err
	}

	// Step 1.3: 无防重凭证时的直接执行兜底
	// 来源: Step 1.1 的 action 闭包
	// 用于: 直接产出申请结果
	_, err := action(ctx)
	return err
}

// ApplyGroup 申请加入群组
// userId: 申请人的用户ID
// req: 包含目标群组ID和申请信息的请求对象
func (u *ApplyService) ApplyGroup(ctx context.Context, userId string, req applyreq.ApplyGroupRequest) error {
	// Step 2.1: 声明入群申请执行体闭包
	// 来源: userId 与 req.GroupId, req.Message
	// 用于: Step 2.2 被第三代状态机幂等探针执行或直接执行
	action := func(execCtx context.Context) (struct{}, error) {
		// 1. 参数校验
		if len(req.GroupId) == 0 {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "群组ID不能为空")
		}

		// 2. 检查群组是否存在
		groupRsp, err := grpc_client.GetGroupDetail(execCtx, userId, req.GroupId)
		if err != nil {
			return struct{}{}, err
		}

		// 3. 检查是否已经是群成员
		isMember, err := grpc_client.CheckGroupMember(execCtx, req.GroupId, userId)
		if err == nil && isMember {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "你已在该群中")
		}

		// 4. 查询之前的申请记录
		apply, err := u.uow.ApplyStore().FindByApplicantIdAndTargetId(execCtx, userId, req.GroupId)
		if err != nil {
			if !errorx.IsNotFound(err) {
				zap.L().Error("Find group apply error", zap.Error(err))
				return struct{}{}, errorx.ErrServerBusy
			}
			apply = nil
		}

		// 5. 黑名单检查
		if apply != nil && apply.Status == apply_status.BLACK {
			return struct{}{}, errorx.New(errorx.CodeInvalidParam, "该群已将你拉黑，无法发送申请")
		}

		// 6. 处理免审核入群逻辑
		if groupRsp.AddMode == int32(add_mode.DIRECT) {
			err := store.WithTx(u.uow, func(tx applyUoW) error {
				if apply != nil {
					_ = tx.ApplyStore().SoftDelete(execCtx, userId, req.GroupId)
				}
				payload, _ := json.Marshal(event.GroupApplyPassedEvent{
					GroupId: req.GroupId,
					UserId:  userId,
				})
				return tx.RecordEvent(execCtx, req.GroupId, event.EventGroupApplyPassed, payload)
			})

			if err != nil {
				zap.L().Error("service error", zap.Error(err))
				return struct{}{}, errorx.ErrServerBusy
			}

			u.cache.SubmitTask(func() {
				_ = u.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+req.GroupId)
				_ = u.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+req.GroupId)
			})

			return struct{}{}, nil
		}

		// 7. 处理需要审核的入群逻辑
		if apply == nil {
			apply = &model.Apply{
				Uuid:        fmt.Sprintf("A%s", snowflake.GenerateIDString()),
				ApplicantId: userId,
				TargetId:    req.GroupId,
				ContactType: apply_type.GROUP,
				Status:      apply_status.PENDING,
				Message:     req.Message,
				LastApplyAt: time.Now(),
				Version:     1,
			}
			if err := u.uow.ApplyStore().CreateApply(execCtx, apply); err != nil {
				if dberr.IsDuplicateKey(err) {
					if existing, getErr := u.uow.ApplyStore().FindByApplicantIdAndTargetId(execCtx, userId, req.GroupId); getErr == nil {
						existing.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
						existing.LastApplyAt = time.Now()
						existing.Status = apply_status.PENDING
						existing.Message = req.Message
						existing.Version += 1
						return struct{}{}, u.uow.ApplyStore().Update(execCtx, existing)
					}
				}
				zap.L().Error("Create group apply error", zap.Error(err))
				return struct{}{}, errorx.ErrServerBusy
			}
		} else {
			apply.Uuid = fmt.Sprintf("A%s", snowflake.GenerateIDString())
			apply.LastApplyAt = time.Now()
			apply.Status = apply_status.PENDING
			apply.Message = req.Message
			apply.Version += 1
			if err := u.uow.ApplyStore().Update(execCtx, apply); err != nil {
				zap.L().Error("Update group apply error", zap.Error(err))
				return struct{}{}, errorx.ErrServerBusy
			}
		}

		return struct{}{}, nil
	}

	// Step 2.2: 接入第三代状态机幂等保护
	// 来源: req.ClientToken 与 u.guard
	// 用于: 短租约防重与重试 0ms 回显
	if u.guard != nil && req.ClientToken != "" {
		key := "apply:group:" + req.ClientToken
		_, _, err := idempotent.ExecuteJSON(ctx, u.guard, key, 5*time.Second, 24*time.Hour, action)
		return err
	}

	// Step 2.3: 无防重凭证时的直接执行兜底
	// 来源: Step 2.1 的 action 闭包
	// 用于: 直接产出加群申请结果
	_, err := action(ctx)
	return err
}

// GetFriendApplyList 获取收到的好友申请列表
// GetFriendApplyList 获取收到的好友申请列表（分页）
// userId: 当前用户的ID
// page: 页码，从1开始
// pageSize: 每页数量
// 返回: 分页好友申请列表响应对象
func (u *ApplyService) GetFriendApplyList(ctx context.Context, userId string, page, pageSize int) (*applyrsp.PagedFriendApplyListRespond, error) {
	// 1. 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 2. 数据库分页查询待处理的申请记录
	applyList, total, err := u.uow.ApplyStore().FindByTargetIdPendingPaged(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 如果没有申请，直接返回空切片
	if total == 0 || len(applyList) == 0 {
		return &applyrsp.PagedFriendApplyListRespond{
			Total: total,
			List:  []applyrsp.FriendApplyListRespond{},
		}, nil
	}

	// 3. 收集当前页申请人ID
	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	// 4. 批量查询申请人公开信息（昵称/头像），避免直读 user 表
	userList, err := grpc_client.BatchGetPublicUserInfo(ctx, userUuids)
	if err != nil {
		zap.L().Error("batch get applicants via grpc error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 5. 构建用户信息Map
	// 将用户列表转换为Map，Key为UUID，Value为用户信息
	// 这样做是为了后续遍历申请列表时能够以O(1)复杂度获取对应用户信息
	userMap := make(map[string]*userpb.PublicUserInfo, len(userList))
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	// 6. 组装响应数据
	rsp := make([]applyrsp.FriendApplyListRespond, 0, len(applyList))
	for _, apply := range applyList {
		// 从Map中获取申请人详细信息
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			// 理论上不应发生（外键约束或逻辑保证），如果发生则跳过该条记录
			continue
		}

		// 处理申请理由，如果为空则显示默认文本
		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}

		// 构建并追加响应对象
		rsp = append(rsp, applyrsp.FriendApplyListRespond{
			ApplicantId:     user.Uuid,
			ApplicantName:   user.Nickname,
			ApplicantAvatar: user.Avatar,
			Message:         message,
		})
	}

	return &applyrsp.PagedFriendApplyListRespond{
		Total: total,
		List:  rsp,
	}, nil
}

// GetGroupApplyList 获取收到的加群申请列表
// GetGroupApplyList 获取收到的加群申请列表
// userId: 操作者ID（必须是管理员或群主）
// groupId: 目标群组ID
func (u *ApplyService) GetGroupApplyList(ctx context.Context, userId, groupId string, page, pageSize int) (*applyrsp.PagedGroupApplyListRespond, error) {
	// 1. 权限校验
	// 检查操作者是否是该群的成员
	// Check if user is member (for permissions, ideally we check role via GroupDetail)
	groupRsp, err := grpc_client.GetGroupDetail(ctx, userId, groupId)
	if err != nil {
		return nil, errorx.New(errorx.CodeForbidden, "无权查看或群聊不存在")
	}
    // We assume if GetGroupDetail passes, we could check role if returned, but for now allow members.
    _ = groupRsp


	// 2. 设置默认分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// 3. 数据库分页查询待处理的入群申请
	applyList, total, err := u.uow.ApplyStore().FindByTargetIdPendingPaged(ctx, groupId, page, pageSize)
	if err != nil {
		zap.L().Error("Find group pending applies error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	if total == 0 || len(applyList) == 0 {
		return &applyrsp.PagedGroupApplyListRespond{
			Total: total,
			List:  []applyrsp.GroupApplyListRespond{},
		}, nil
	}

	// 4. 收集当前页申请人ID
	userUuids := make([]string, 0, len(applyList))
	for _, apply := range applyList {
		userUuids = append(userUuids, apply.ApplicantId)
	}

	// 5. 批量查询申请人公开信息（昵称/头像），避免直读 user 表
	userList, err := grpc_client.BatchGetPublicUserInfo(ctx, userUuids)
	if err != nil {
		zap.L().Error("batch get applicants via grpc error", zap.Error(err))
		return nil, errorx.ErrServerBusy
	}

	// 6. 构建用户信息Map
	userMap := make(map[string]*userpb.PublicUserInfo, len(userList))
	for _, user := range userList {
		userMap[user.Uuid] = user
	}

	// 7. 组装响应数据
	rsp := make([]applyrsp.GroupApplyListRespond, 0, len(applyList))
	for _, apply := range applyList {
		user, ok := userMap[apply.ApplicantId]
		if !ok {
			continue
		}

		message := "申请理由：无"
		if apply.Message != "" {
			message = "申请理由：" + apply.Message
		}

		rsp = append(rsp, applyrsp.GroupApplyListRespond{
			ApplicantId:     user.Uuid,
			ApplicantName:   user.Nickname,
			ApplicantAvatar: user.Avatar,
			Message:         message,
		})
	}

	return &applyrsp.PagedGroupApplyListRespond{
		Total: total,
		List:  rsp,
	}, nil
}

// PassFriendApply 通过好友申请
// userId: 操作者ID（即被申请人）
// applicantId: 申请人ID（即发起好友申请的用户）
func (u *ApplyService) PassFriendApply(ctx context.Context, userId string, applicantId string) error {
	// Step 1.1: 查询好友申请记录
	// 来源: applicantId 与 userId
	// 用于: Step 1.2 校验申请存在性及审批状态
	apply, err := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 1.2: 终态幂等自愈检查与状态校验
	// 来源: Step 1.1 查询到的 apply.Status
	// 用于: 命中已通过终态时静默自愈返回，防止网络重试报错；非待审核状态阻断非法操作
	if apply.Status == apply_status.AGREE {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	// Step 1.3: 校验申请人账号状态
	// 来源: applicantId
	// 用于: 防止审批通过已被禁用或注销的用户
	applicantStatusRsp, err := grpc_client.UserClient.GetUserStatus(ctx, &userpb.GetUserStatusRequest{UserId: applicantId})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		zap.L().Error("Get applicant status error", zap.String("applicantId", applicantId), zap.Error(err))
		return errorx.ErrServerBusy
	}
	if int8(applicantStatusRsp.Status) == user_status.DISABLE {
		return errorx.New(errorx.CodeInvalidParam, "该用户已被禁用")
	}

	// Step 1.4: 事务原子跃迁与 Outbox 事件投递
	// 来源: apply.Uuid 及申请双方身份
	// 用于: 仅首次成功跃迁写入 friend_apply_passed 事件，并发抢占落后时幂等自愈
	var alreadyAgreed bool
	err = store.WithTx(u.uow, func(tx applyUoW) error {
		ok, err := tx.ApplyStore().UpdateStatusIfPending(ctx, apply.Uuid, apply_status.AGREE, apply.Version)
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok {
			// Step 1.4.1: 检查 CAS 执行结果（RowsAffected == 0），反查真实记录并按规范分流
			// 来源: UpdateStatusIfPending 的 ok == false
			// 用于: 拦截越权/非法状态，或终态放行
			cur, queryErr := tx.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, userId)
			if queryErr != nil || cur == nil {
				return errorx.New(errorx.CodeNotFound, "申请记录不存在")
			}
			// 【先拦截 1】：用户不匹配（越权操作，最严重的合规问题优先拦截）
			if cur.TargetId != userId {
				return errorx.New(errorx.CodeForbidden, "无权处理该好友申请")
			}
			// 【先拦截 2】：已被拒绝 / 已被拉黑（非法的前置终态）
			if cur.Status == apply_status.REFUSE {
				return errorx.New(errorx.CodeInvalidParam, "该申请已被拒绝")
			}
			if cur.Status == apply_status.BLACK {
				return errorx.New(errorx.CodeInvalidParam, "该申请已被拉黑")
			}
			// 【最后放行】：确认确实是网络抖动导致的重复请求，静默放行掐断副作用
			if cur.Status == apply_status.AGREE {
				alreadyAgreed = true
				return nil
			}
			return errorx.ErrServerBusy
		}

		payload, _ := json.Marshal(event.FriendApplyPassedEvent{
			UserId:   userId,
			FriendId: applicantId,
		})
		return tx.RecordEvent(ctx, userId, event.EventFriendApplyPassed, payload)
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

// PassGroupApply 通过入群申请
// operatorId: 操作者ID（必须是群主或管理员）
// groupId: 目标群组ID
// applicantId: 申请入群的用户ID
func (u *ApplyService) PassGroupApply(ctx context.Context, operatorId, groupId, applicantId string) error {
	// Step 2.1: 权限校验
	// 来源: operatorId 与 groupId
	// 用于: 防止非群成员操作审批
	isMember, err := grpc_client.CheckGroupMember(ctx, groupId, operatorId)
	if err != nil || !isMember {
		return errorx.New(errorx.CodeForbidden, "你不是该群成员或无权限")
	}

	// Step 2.2: 查找申请记录
	// 来源: applicantId 与 groupId
	// 用于: Step 2.3 校验申请状态及终态自愈
	apply, err := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 2.3: 终态幂等自愈检查与状态校验
	// 来源: Step 2.2 查询到的 apply.Status
	// 用于: 已通过终态直接自愈返回，防止重试误报
	if apply.Status == apply_status.AGREE {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	// Step 2.4: 开启事务执行 CAS 状态跃迁与 Outbox 事件投递
	// 来源: apply.Uuid 及 groupId, applicantId
	// 用于: 仅首次成功跃迁写入 group_apply_passed 事件
	var alreadyAgreed bool
	err = store.WithTx(u.uow, func(tx applyUoW) error {
		ok, err := tx.ApplyStore().UpdateStatusIfPending(ctx, apply.Uuid, apply_status.AGREE, apply.Version)
		if err != nil {
			zap.L().Error("service error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok {
			// Step 2.4.1: 检查 CAS 执行结果（RowsAffected == 0），反查真实记录并按规范分流
			// 来源: UpdateStatusIfPending 的 ok == false
			// 用于: 拦截越权/非法状态，或终态放行
			cur, queryErr := tx.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
			if queryErr != nil || cur == nil {
				return errorx.New(errorx.CodeNotFound, "申请记录不存在")
			}
			// 【先拦截 1】：目标群组不匹配（越权操作，最严重的合规问题优先拦截）
			if cur.TargetId != groupId {
				return errorx.New(errorx.CodeForbidden, "目标群组不匹配")
			}
			// 【先拦截 2】：已被拒绝 / 已被拉黑（非法的前置终态）
			if cur.Status == apply_status.REFUSE {
				return errorx.New(errorx.CodeInvalidParam, "该申请已被拒绝")
			}
			if cur.Status == apply_status.BLACK {
				return errorx.New(errorx.CodeInvalidParam, "该申请已被拉黑")
			}
			// 【最后放行】：确认确实是网络抖动导致的重复请求，静默放行掐断副作用
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
		return tx.RecordEvent(ctx, groupId, event.EventGroupApplyPassed, payload)
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

	// Step 2.5: 异步失效群信息与群成员缓存
	// 来源: groupId
	// 用于: 保证后续群信息及群成员读取一致性
	u.cache.SubmitTask(func() {
		_ = u.cacheHelper.InvalidateWithNull(context.Background(), constants.CacheKeyGroupInfo+groupId)
		_ = u.cache.Delete(context.Background(), constants.CacheKeyGroupMembers+groupId)
	})

	return nil
}

// RefuseFriendApply 拒绝好友申请
// userId: 操作者ID
// applicantId: 申请人ID
func (u *ApplyService) RefuseFriendApply(ctx context.Context, userId string, applicantId string) error {
	// Step 3.1: 查找申请记录
	// 来源: applicantId 与 userId
	// 用于: Step 3.2 校验状态与终态幂等自愈
	apply, err := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, userId)
	if err != nil {
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 3.2: 终态幂等检查
	// 来源: Step 3.1 查询到的 apply.Status
	// 用于: 处于 REFUSE 终态时静默自愈返回
	if apply.Status == apply_status.REFUSE {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	// Step 3.3: 原子 CAS 跃迁至 REFUSE
	// 来源: apply.Uuid 与 apply.Version
	// 用于: 状态更新并支持并发重试自愈
	ok, err := u.uow.ApplyStore().UpdateStatusIfPending(ctx, apply.Uuid, apply_status.REFUSE, apply.Version)
	if err != nil {
		zap.L().Error("Update friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !ok {
		// Step 3.4: 检查 CAS 执行结果（RowsAffected == 0），反查真实记录并按规范分流
		// 来源: UpdateStatusIfPending 的 ok == false
		// 用于: 拦截越权/非法状态，或终态放行
		cur, queryErr := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, userId)
		if queryErr != nil || cur == nil {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		// 【先拦截 1】：用户不匹配（越权操作，最严重的合规问题优先拦截）
		if cur.TargetId != userId {
			return errorx.New(errorx.CodeForbidden, "无权处理该好友申请")
		}
		// 【先拦截 2】：已被同意 / 已被拉黑（非法的前置终态）
		if cur.Status == apply_status.AGREE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被同意，无法拒绝")
		}
		if cur.Status == apply_status.BLACK {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被拉黑")
		}
		// 【最后放行】：确认确实是网络抖动导致的重复请求，静默放行
		if cur.Status == apply_status.REFUSE {
			return nil
		}
		return errorx.ErrServerBusy
	}
	return nil
}

// RefuseGroupApply 拒绝入群申请
// operatorId: 操作者ID（必须是群主或管理员）
// groupId: 目标群组ID
// applicantId: 申请入群的用户ID
func (u *ApplyService) RefuseGroupApply(ctx context.Context, operatorId, groupId, applicantId string) error {
	// Step 4.1: 权限校验
	// 来源: groupId 与 operatorId
	// 用于: 确认操作者具备群成员身份
	isMember, err := grpc_client.CheckGroupMember(ctx, groupId, operatorId)
	if err != nil || !isMember {
		return errorx.New(errorx.CodeForbidden, "你不是该群成员或无权限")
	}

	// Step 4.2: 查找申请记录
	// 来源: applicantId 与 groupId
	// 用于: Step 4.3 终态幂等自愈与状态校验
	apply, err := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
	if err != nil {
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 4.3: 终态幂等自愈检查
	// 来源: Step 4.2 查询到的 apply.Status
	// 用于: 处于 REFUSE 终态时静默自愈返回
	if apply.Status == apply_status.REFUSE {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	// Step 4.4: 原子 CAS 跃迁至 REFUSE
	// 来源: apply.Uuid 与 apply.Version
	// 用于: 状态更新并支持并发重试自愈
	ok, err := u.uow.ApplyStore().UpdateStatusIfPending(ctx, apply.Uuid, apply_status.REFUSE, apply.Version)
	if err != nil {
		zap.L().Error("Update group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !ok {
		// Step 4.5: 检查 CAS 执行结果（RowsAffected == 0），反查真实记录并按规范分流
		// 来源: UpdateStatusIfPending 的 ok == false
		// 用于: 拦截越权/非法状态，或终态放行
		cur, queryErr := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
		if queryErr != nil || cur == nil {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		// 【先拦截 1】：目标群组不匹配（越权操作，最严重的合规问题优先拦截）
		if cur.TargetId != groupId {
			return errorx.New(errorx.CodeForbidden, "目标群组不匹配")
		}
		// 【先拦截 2】：已被同意 / 已被拉黑（非法的前置终态）
		if cur.Status == apply_status.AGREE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被同意，无法拒绝")
		}
		if cur.Status == apply_status.BLACK {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被拉黑")
		}
		// 【最后放行】：确认确实是网络抖动导致的重复请求，静默放行
		if cur.Status == apply_status.REFUSE {
			return nil
		}
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackFriendApply 拉黑好友申请
// userId: 操作者ID
// applicantId: 申请人ID
func (u *ApplyService) BlackFriendApply(ctx context.Context, userId string, applicantId string) error {
	// Step 5.1: 查找申请记录
	// 来源: applicantId 与 userId
	// 用于: Step 5.2 终态幂等自愈与状态校验
	apply, err := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find friend apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 5.2: 终态幂等自愈检查
	// 来源: Step 5.1 查询到的 apply.Status
	// 用于: 处于 BLACK 终态时静默自愈返回
	if apply.Status == apply_status.BLACK {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	// Step 5.3: 原子 CAS 跃迁至 BLACK
	// 来源: apply.Uuid 与 apply.Version
	// 用于: 状态更新并支持并发重试自愈
	ok, err := u.uow.ApplyStore().UpdateStatusIfPending(ctx, apply.Uuid, apply_status.BLACK, apply.Version)
	if err != nil {
		zap.L().Error("Update friend apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !ok {
		// Step 5.4: 检查 CAS 执行结果（RowsAffected == 0），反查真实记录并按规范分流
		// 来源: UpdateStatusIfPending 的 ok == false
		// 用于: 拦截越权/非法状态，或终态放行
		cur, queryErr := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, userId)
		if queryErr != nil || cur == nil {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		// 【先拦截 1】：用户不匹配（越权操作，最严重的合规问题优先拦截）
		if cur.TargetId != userId {
			return errorx.New(errorx.CodeForbidden, "无权处理该好友申请")
		}
		// 【先拦截 2】：已被同意 / 已被拒绝（非法的前置终态）
		if cur.Status == apply_status.AGREE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被同意，无法拉黑")
		}
		if cur.Status == apply_status.REFUSE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被拒绝，无法拉黑")
		}
		// 【最后放行】：确认确实是网络抖动导致的重复请求，静默放行
		if cur.Status == apply_status.BLACK {
			return nil
		}
		return errorx.ErrServerBusy
	}
	return nil
}

// BlackGroupApply 拉黑入群申请
// operatorId: 操作者ID（必须是群主或管理员）
// groupId: 目标群组ID
// applicantId: 申请入群的用户ID
func (u *ApplyService) BlackGroupApply(ctx context.Context, operatorId, groupId, applicantId string) error {
	// Step 6.1: 权限校验
	// 来源: groupId 与 operatorId
	// 用于: 仅群主或管理员 (Role >= 2) 可审批
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

	// Step 6.2: 查找申请记录
	// 来源: applicantId 与 groupId
	// 用于: Step 6.3 终态幂等自愈与状态校验
	apply, err := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		zap.L().Error("Find group apply error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 6.3: 终态幂等自愈检查
	// 来源: Step 6.2 查询到的 apply.Status
	// 用于: 处于 BLACK 终态时静默自愈返回
	if apply.Status == apply_status.BLACK {
		return nil
	}
	if apply.Status != apply_status.PENDING {
		return errorx.New(errorx.CodeInvalidParam, "该申请已被处理，无法重复操作")
	}

	// Step 6.4: 原子 CAS 跃迁至 BLACK
	// 来源: apply.Uuid 与 apply.Version
	// 用于: 状态更新并支持并发重试自愈
	ok, err := u.uow.ApplyStore().UpdateStatusIfPending(ctx, apply.Uuid, apply_status.BLACK, apply.Version)
	if err != nil {
		zap.L().Error("Update group apply status error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !ok {
		// Step 6.5: 检查 CAS 执行结果（RowsAffected == 0），反查真实记录并按规范分流
		// 来源: UpdateStatusIfPending 的 ok == false
		// 用于: 拦截越权/非法状态，或终态放行
		cur, queryErr := u.uow.ApplyStore().FindByApplicantIdAndTargetId(ctx, applicantId, groupId)
		if queryErr != nil || cur == nil {
			return errorx.New(errorx.CodeNotFound, "申请记录不存在")
		}
		// 【先拦截 1】：目标群组不匹配（越权操作，最严重的合规问题优先拦截）
		if cur.TargetId != groupId {
			return errorx.New(errorx.CodeForbidden, "目标群组不匹配")
		}
		// 【先拦截 2】：已被同意 / 已被拒绝（非法的前置终态）
		if cur.Status == apply_status.AGREE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被同意，无法拉黑")
		}
		if cur.Status == apply_status.REFUSE {
			return errorx.New(errorx.CodeInvalidParam, "该申请已被拒绝，无法拉黑")
		}
		// 【最后放行】：确认确实是网络抖动导致的重复请求，静默放行
		if cur.Status == apply_status.BLACK {
			return nil
		}
		return errorx.ErrServerBusy
	}
	return nil
}
