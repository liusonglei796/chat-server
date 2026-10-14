package friendship

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/common/dao/mysql/dberr"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	friendshiprsp "kama_chat_server/internal/common/dto/respond/friendship"
	userrsp "kama_chat_server/internal/common/dto/respond/user"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
)

// friendshipUoW 好友关系服务的 UoW 接口：事务/事件能力 + 仅本服务拥有的 Friendship Store
type friendshipUoW interface {
	store.TxExecutor
	RecordEvent(ctx context.Context, aggregateId, eventType string, payload []byte) error
	FriendshipStore() store.FriendshipStore
}

// FriendshipService 好友关系业务逻辑实现
type FriendshipService struct {
	uow   friendshipUoW
	cache store.AsyncCacheService
}

// NewFriendshipService 构造函数，注入所有依赖
func NewFriendshipService(uow friendshipUoW, cacheService store.AsyncCacheService) *FriendshipService {
	return &FriendshipService{
		uow:   uow,
		cache: cacheService,
	}
}

// clearFriendRelationCache 清理好友关系相关的缓存（DRY: DeleteFriend/BlackFriend 共用）
func (s *FriendshipService) clearFriendRelationCache(userId, friendId string) {
	s.cache.SubmitTask(func() {
		//从好友set中移除一个好友
		_ = s.cache.RemoveFromSet(context.Background(), constants.CacheKeyFriendRelUser+userId, friendId)
		_ = s.cache.RemoveFromSet(context.Background(), constants.CacheKeyFriendRelUser+friendId, userId)
	})
}

// GetFriendList 获取好友列表（分页）
func (s *FriendshipService) GetFriendList(ctx context.Context, userId string, page, pageSize int) ([]userrsp.MyUserListRespond, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 从数据库分页查询好友关系
	friendships, total, err := s.uow.FriendshipStore().FindFriendsByUserId(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find friendship list error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	if len(friendships) == 0 {
		return []userrsp.MyUserListRespond{}, total, nil
	}

	// 批量获取好友公开信息（昵称/头像），避免逐条直读 user 表
	friendIds := make([]string, 0, len(friendships))
	for _, fs := range friendships {
		friendIds = append(friendIds, fs.FriendId)
	}
	userList, err := grpc_client.BatchGetPublicUserInfo(ctx, friendIds)
	if err != nil {
		zap.L().Error("batch get friends info via grpc error", zap.Error(err))
		return []userrsp.MyUserListRespond{}, total, errorx.ErrServerBusy
	}
	userMap := make(map[string]*userpb.PublicUserInfo, len(userList))
	for _, u := range userList {
		userMap[u.Uuid] = u
	}

	userListRsp := make([]userrsp.MyUserListRespond, 0, len(friendships))
	for _, fs := range friendships {
		if u, ok := userMap[fs.FriendId]; ok {
			userListRsp = append(userListRsp, userrsp.MyUserListRespond{
				UserId:   fs.FriendId,
				UserName: u.Nickname,
				Avatar:   u.Avatar,
			})
		}
	}

	return userListRsp, total, nil
}

// GetFriendInfo 获取好友详情 (userId 必须与 friendId 是好友关系)
func (s *FriendshipService) GetFriendInfo(ctx context.Context, userId, friendId string) (friendshiprsp.FriendInfoRespond, error) {
	if len(friendId) == 0 {
		return friendshiprsp.FriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	isFriend, err := s.uow.FriendshipStore().IsFriend(ctx, userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return friendshiprsp.FriendInfoRespond{}, errorx.ErrServerBusy
	}
	if !isFriend {
		return friendshiprsp.FriendInfoRespond{}, errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	// 校验好友账号状态（通过 user_service 的 GetUserStatus，避免跨服务直读 user 表）
	friendStatusRsp, err := grpc_client.UserClient.GetUserStatus(ctx, &userpb.GetUserStatusRequest{UserId: friendId})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return friendshiprsp.FriendInfoRespond{}, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		zap.L().Error("Get friend status error", zap.String("friendId", friendId), zap.Error(err))
		return friendshiprsp.FriendInfoRespond{}, errorx.ErrServerBusy
	}
	if int8(friendStatusRsp.Status) == user_status.DISABLE {
		return friendshiprsp.FriendInfoRespond{}, errorx.New(errorx.CodeInvalidParam, "该用户处于禁用状态")
	}

	// 获取好友公开信息（昵称/头像/性别/生日/签名），避免直读 user 表
	userInfo, err := grpc_client.GetPublicUserInfo(ctx, friendId)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return friendshiprsp.FriendInfoRespond{}, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		zap.L().Error("get friend info via grpc error", zap.String("friendId", friendId), zap.Error(err))
		return friendshiprsp.FriendInfoRespond{}, errorx.ErrServerBusy
	}

	// 获取好友备注（从 Friendship 记录中获取）
	var remark string
	fs, err := s.uow.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
	if err == nil {
		remark = fs.Remark
	}

	return friendshiprsp.FriendInfoRespond{
		FriendId:        userInfo.Uuid,
		FriendName:      userInfo.Nickname,
		FriendAvatar:    userInfo.Avatar,
		FriendBirthday:  userInfo.Birthday,
		FriendGender:    int8(userInfo.Gender),
		FriendSignature: userInfo.Signature,
		Remark:          remark,
	}, nil
}

// DeleteFriend 删除好友（双向删除）
func (s *FriendshipService) DeleteFriend(ctx context.Context, userId, friendId string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能删除自己")
	}

	isFriend, err := s.uow.FriendshipStore().IsFriend(ctx, userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !isFriend {
		// 终态幂等自愈：若当前已不是好友（前序删除重试已成功），静默放行并阻断重复事件
		return nil
	}

	err = store.WithTx(s.uow, func(tx friendshipUoW) error {
		if err := tx.FriendshipStore().SoftDelete(ctx, userId, friendId); err != nil {
			zap.L().Error("Delete friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 写入 outbox 表 (friend_blacked 事件)，由 Canal CDC 捕获并推至 Kafka 异步解耦
		payload, _ := json.Marshal(event.FriendBlackedEvent{
			UserId:   userId,
			FriendId: friendId,
		})
		return tx.RecordEvent(ctx, userId, event.EventFriendBlacked, payload)
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	s.clearFriendRelationCache(userId, friendId)
	return nil
}

// BlackFriend 拉黑好友（状态机 CAS 原子跃迁）
func (s *FriendshipService) BlackFriend(ctx context.Context, userId string, friendId string, expectedVersion ...int64) error {
	// Step 1.1: 基础参数自检
	// 来源: 客户端请求传入的 userId 与 friendId
	// 用于: 拦截不能拉黑自己的非法请求
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能拉黑自己")
	}

	// Step 1.2: 开启事务执行双向好友状态原子 CAS 跃迁
	// 来源: Step 1.1 校验通过的用户对与 expectedVersion
	// 用于: 原子修改双方状态并可靠写入 Outbox 领域事件
	err := store.WithTx(s.uow, func(tx friendshipUoW) error {
		myFs, err := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeForbidden, "你们还不是好友，无法拉黑")
			}
			zap.L().Error("Find friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 校验客户端预期的版本号，拦截因弱网延迟送达的陈旧 ABA 请求
		if len(expectedVersion) > 0 && expectedVersion[0] > 0 {
			if myFs.Version != expectedVersion[0] {
				return errorx.New(errorx.CodeConflict, "好友状态已被并发修改或请求已过期")
			}
		}
		if myFs.Status != friendship_status.NORMAL {
			return errorx.New(errorx.CodeInvalidParam, "当前状态不允许拉黑")
		}

		theirFs, err := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, friendId, userId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "对方好友关系不存在")
			}
			zap.L().Error("Find reverse friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if theirFs.Status != friendship_status.NORMAL {
			return errorx.New(errorx.CodeInvalidParam, "好友关系状态异常")
		}

		// 全局资源排序法：按 userId 字典序固定加锁更新顺序，杜绝 AB-BA 交叉死锁
		firstUser, secondUser := userId, friendId
		var firstExpected, firstTarget int8 = friendship_status.NORMAL, friendship_status.BLACK
		var secondExpected, secondTarget int8 = friendship_status.NORMAL, friendship_status.BE_BLACK
		firstVersion := myFs.Version
		secondVersion := theirFs.Version
		if firstUser > secondUser {
			firstUser, secondUser = friendId, userId
			firstExpected, firstTarget = friendship_status.NORMAL, friendship_status.BE_BLACK
			secondExpected, secondTarget = friendship_status.NORMAL, friendship_status.BLACK
			firstVersion = theirFs.Version
			secondVersion = myFs.Version
		}
		if len(expectedVersion) > 0 && expectedVersion[0] > 0 {
			if firstUser == userId {
				firstVersion = expectedVersion[0]
			} else {
				secondVersion = expectedVersion[0]
			}
		}

		// 原子 CAS 跃迁第一条记录 (Compare-And-Swap with Version)
		ok1, err := tx.FriendshipStore().UpdateStatusWithExpected(ctx, firstUser, secondUser, firstExpected, firstTarget, firstVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected to first error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok1 {
			// Step 1.2.1: 检查 CAS 执行结果（RowsAffected == 0），反查真实记录并按规范分流
			cur, queryErr := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
			if queryErr != nil || cur == nil {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
			// 【先拦截 1】：用户不匹配（越权操作，最严重的合规问题优先拦截）
			if cur.UserId != userId {
				return errorx.New(errorx.CodeForbidden, "无权操作该好友关系")
			}
			// 【先拦截 2】：已被对方拉黑等非法终态
			if cur.Status == friendship_status.BE_BLACK {
				return errorx.New(errorx.CodeInvalidParam, "已被对方拉黑，无法重复拉黑")
			}
			// 状态或版本已被并发修改，说明已有并发请求抢先拉黑或版本冲突，立即拦截防止重复触发与事件重发
			return errorx.New(errorx.CodeInvalidParam, "当前状态不允许拉黑或已被处理")
		}

		// 原子 CAS 跃迁第二条记录
		ok2, err := tx.FriendshipStore().UpdateStatusWithExpected(ctx, secondUser, firstUser, secondExpected, secondTarget, secondVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected to second error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok2 {
			return errorx.New(errorx.CodeInvalidParam, "好友关系状态异常")
		}

		// 写入 outbox 表 (friend_blacked 事件)，由 Canal CDC 捕获并推至 Kafka 异步解耦
		payload, _ := json.Marshal(event.FriendBlackedEvent{
			UserId:   userId,
			FriendId: friendId,
		})
		return tx.RecordEvent(ctx, userId, event.EventFriendBlacked, payload)
	})

	if err != nil {
		if e, ok := err.(*errorx.CodeError); ok {
			return e
		}
		zap.L().Error("BlackFriend transaction error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 1.3: 清理好友关系本地缓存
	// 来源: Step 1.2 成功完成的拉黑事务
	// 用于: 失效双方好友关系缓存
	s.clearFriendRelationCache(userId, friendId)
	return nil
}

// UnblackFriend 取消拉黑好友（状态机 CAS 原子跃迁）
func (s *FriendshipService) UnblackFriend(ctx context.Context, userId string, friendId string, expectedVersion ...int64) error {
	// Step 2.1: 基础参数自检
	// 来源: 客户端请求传入的 userId 与 friendId
	// 用于: 拦截非法参数
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "参数错误")
	}

	// Step 2.2: 开启事务执行解除拉黑状态机原子 CAS 跃迁
	// 来源: Step 2.1 校验通过的用户对与 expectedVersion
	// 用于: 原子恢复双向关系为 NORMAL 状态
	err := store.WithTx(s.uow, func(tx friendshipUoW) error {
		myFs, err := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
			zap.L().Error("Find friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		// 校验客户端预期的版本号，拦截因弱网延迟送达的陈旧 ABA 请求
		if len(expectedVersion) > 0 && expectedVersion[0] > 0 {
			if myFs.Version != expectedVersion[0] {
				return errorx.New(errorx.CodeConflict, "好友状态已被并发修改或请求已过期")
			}
		}
		if myFs.Status != friendship_status.BLACK {
			return errorx.New(errorx.CodeInvalidParam, "未拉黑该好友，无需解除拉黑")
		}

		theirFs, err := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, friendId, userId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "对方好友关系不存在")
			}
			zap.L().Error("Find reverse friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if theirFs.Status != friendship_status.BE_BLACK {
			return errorx.New(errorx.CodeInvalidParam, "数据状态异常，请联系管理员")
		}

		// 全局资源排序法：按 userId 字典序固定加锁更新顺序，杜绝 AB-BA 交叉死锁
		firstUser, secondUser := userId, friendId
		var firstExpected, firstTarget int8 = friendship_status.BLACK, friendship_status.NORMAL
		var secondExpected, secondTarget int8 = friendship_status.BE_BLACK, friendship_status.NORMAL
		firstVersion := myFs.Version
		secondVersion := theirFs.Version
		if firstUser > secondUser {
			firstUser, secondUser = friendId, userId
			firstExpected, firstTarget = friendship_status.BE_BLACK, friendship_status.NORMAL
			secondExpected, secondTarget = friendship_status.BLACK, friendship_status.NORMAL
			firstVersion = theirFs.Version
			secondVersion = myFs.Version
		}
		if len(expectedVersion) > 0 && expectedVersion[0] > 0 {
			if firstUser == userId {
				firstVersion = expectedVersion[0]
			} else {
				secondVersion = expectedVersion[0]
			}
		}

		ok1, err := tx.FriendshipStore().UpdateStatusWithExpected(ctx, firstUser, secondUser, firstExpected, firstTarget, firstVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected first error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok1 {
			// Step 2.2.1: 检查 CAS 执行结果（RowsAffected == 0），反查真实记录并按规范分流
			cur, queryErr := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
			if queryErr != nil || cur == nil {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
			// 【先拦截 1】：用户不匹配（越权操作，最严重的合规问题优先拦截）
			if cur.UserId != userId {
				return errorx.New(errorx.CodeForbidden, "无权操作该好友关系")
			}
			return errorx.New(errorx.CodeInvalidParam, "好友关系状态已变更，无需重复操作")
		}

		ok2, err := tx.FriendshipStore().UpdateStatusWithExpected(ctx, secondUser, firstUser, secondExpected, secondTarget, secondVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected second error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok2 {
			return errorx.New(errorx.CodeInvalidParam, "好友关系状态异常")
		}

		return nil
	})

	if err != nil {
		if e, ok := err.(*errorx.CodeError); ok {
			return e
		}
		zap.L().Error("UnblackFriend transaction error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	// Step 2.3: 异步回写恢复好友关系缓存
	// 来源: Step 2.2 成功提交的解除拉黑事务
	// 用于: 重新将双方加入 Redis 好友关系集合
	s.cache.SubmitTask(func() {
		_ = s.cache.AddToSet(context.Background(), constants.CacheKeyFriendRelUser+userId, friendId)
		_ = s.cache.AddToSet(context.Background(), constants.CacheKeyFriendRelUser+friendId, userId)
	})

	return nil
}

// UpdateRemark 更新好友备注
func (s *FriendshipService) UpdateRemark(ctx context.Context, userId, friendId, remark string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能给自己设置备注")
	}

	// 校验好友关系
	isFriend, err := s.uow.FriendshipStore().IsFriend(ctx, userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !isFriend {
		return errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	if err := s.uow.FriendshipStore().UpdateRemark(ctx, userId, friendId, remark); err != nil {
		zap.L().Error("Update remark error",
			zap.String("user_id", userId),
			zap.String("friend_id", friendId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	return nil
}

// GetFriendshipStatus 返回好友关系状态（对外契约）
// 0=非好友 1=正常 2=已拉黑对方 3=被对方拉黑
func (s *FriendshipService) GetFriendshipStatus(ctx context.Context, userId, friendId string) (int8, error) {
	fs, err := s.uow.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return 0, nil
		}
		zap.L().Error("query friendship error", zap.Error(err))
		return 0, errorx.ErrServerBusy
	}
	switch fs.Status {
	case friendship_status.BE_BLACK: // 被对方拉黑
		return 3, nil
	case friendship_status.BLACK: // 拉黑对方
		return 2, nil
	default: // NORMAL
		return 1, nil
	}
}

// EstablishFriendship 跨服务建立双向好友关系（供加好友审批通过后同步 RPC 调用）
func (s *FriendshipService) EstablishFriendship(ctx context.Context, userId, friendId string) error {
	if userId == "" || friendId == "" || userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "参数错误")
	}

	err := store.WithTx(s.uow, func(tx friendshipUoW) error {
		newFriendship := model.Friendship{
			UserId:   userId,
			FriendId: friendId,
			Status:   friendship_status.NORMAL,
			Version:  1,
		}
		if err := tx.FriendshipStore().CreateFriendship(ctx, &newFriendship); err != nil {
			if !dberr.IsDuplicateKey(err) {
				return err
			}
		}

		anotherFriendship := model.Friendship{
			UserId:   friendId,
			FriendId: userId,
			Status:   friendship_status.NORMAL,
			Version:  1,
		}
		if err := tx.FriendshipStore().CreateFriendship(ctx, &anotherFriendship); err != nil {
			if !dberr.IsDuplicateKey(err) {
				return err
			}
		}
		return nil
	})

	if err != nil {
		zap.L().Error("EstablishFriendship transaction failed",
			zap.String("userId", userId),
			zap.String("friendId", friendId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	s.clearFriendRelationCache(userId, friendId)
	return nil
}
