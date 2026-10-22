package friendship

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "kama_chat_server/api/gen/user"
	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/event"
)

// FriendshipService 好友关系业务逻辑实现
type FriendshipService struct {
	uow   UoW
	cache cacheinfra.AsyncCacheService
}

// NewFriendshipService 构造函数，注入所有依赖
func NewFriendshipService(uow UoW, cacheService ...cacheinfra.AsyncCacheService) *FriendshipService {
	var c cacheinfra.AsyncCacheService
	if len(cacheService) > 0 {
		c = cacheService[0]
	}
	return &FriendshipService{
		uow:   uow,
		cache: c,
	}
}

// GetFriendList 获取好友列表（分页）
func (s *FriendshipService) GetFriendList(ctx context.Context, userId string, page, pageSize int) ([]FriendItem, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	friendships, total, err := s.uow.FriendshipStore().FindFriendsByUserId(ctx, userId, page, pageSize)
	if err != nil {
		zap.L().Error("Find friendship list error", zap.Error(err))
		return nil, 0, errorx.ErrServerBusy
	}

	if len(friendships) == 0 {
		return []FriendItem{}, total, nil
	}

	friendIds := make([]string, 0, len(friendships))
	for _, fs := range friendships {
		friendIds = append(friendIds, fs.FriendId)
	}
	userList, err := grpc_client.BatchGetPublicUserInfo(ctx, friendIds)
	if err != nil {
		zap.L().Error("batch get friends info via grpc error", zap.Error(err))
		return []FriendItem{}, total, errorx.ErrServerBusy
	}
	userMap := make(map[string]*userpb.PublicUserInfo, len(userList))
	for _, u := range userList {
		userMap[u.Uuid] = u
	}

	userListRsp := make([]FriendItem, 0, len(friendships))
	for _, fs := range friendships {
		if u, ok := userMap[fs.FriendId]; ok {
			userListRsp = append(userListRsp, FriendItem{
				UserId:   fs.FriendId,
				UserName: u.Nickname,
				Avatar:   u.Avatar,
			})
		}
	}

	return userListRsp, total, nil
}

// GetFriendInfo 获取好友详情 (userId 必须与 friendId 是好友关系)
func (s *FriendshipService) GetFriendInfo(ctx context.Context, userId, friendId string) (FriendInfo, error) {
	if len(friendId) == 0 {
		return FriendInfo{}, errorx.New(errorx.CodeInvalidParam, "好友ID不能为空")
	}

	isFriend, err := s.uow.FriendshipStore().IsFriend(ctx, userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return FriendInfo{}, errorx.ErrServerBusy
	}
	if !isFriend {
		return FriendInfo{}, errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	friendStatusRsp, err := grpc_client.UserClient.GetUserStatus(ctx, &userpb.GetUserStatusRequest{UserId: friendId})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return FriendInfo{}, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		zap.L().Error("Get friend status error", zap.String("friendId", friendId), zap.Error(err))
		return FriendInfo{}, errorx.ErrServerBusy
	}
	if int8(friendStatusRsp.Status) == user_status.DISABLE {
		return FriendInfo{}, errorx.New(errorx.CodeInvalidParam, "该用户处于禁用状态")
	}

	userInfo, err := grpc_client.GetPublicUserInfo(ctx, friendId)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return FriendInfo{}, errorx.New(errorx.CodeUserNotExist, "该用户不存在")
		}
		zap.L().Error("get friend info via grpc error", zap.String("friendId", friendId), zap.Error(err))
		return FriendInfo{}, errorx.ErrServerBusy
	}

	var remark string
	fs, err := s.uow.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
	if err == nil {
		remark = fs.Remark
	}

	return FriendInfo{
		FriendId:        userInfo.Uuid,
		FriendName:      userInfo.Nickname,
		FriendAvatar:    userInfo.Avatar,
		FriendBirthday:  userInfo.Birthday,
		FriendGender:    int8(userInfo.Gender),
		FriendSignature: userInfo.Signature,
		Remark:          remark,
	}, nil
}

// DeleteFriend 删除好友（微信同款单向删除：仅删除本人与对方的关系记录，对方仍保留单向好友）
func (s *FriendshipService) DeleteFriend(ctx context.Context, userId, friendId string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能删除自己")
	}

	fs, err := s.uow.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil
		}
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if fs == nil {
		return nil
	}

	err = WithTx(s.uow, func(tx UoW) error {
		if err := tx.FriendshipStore().SoftDelete(ctx, userId, friendId); err != nil {
			zap.L().Error("Delete friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		payload, _ := json.Marshal(event.FriendDeletedEvent{
			UserId:   userId,
			FriendId: friendId,
		})
		return tx.RecordEvent(ctx, userId, event.EventFriendDeleted, payload)
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	return nil
}

// BlackFriend 拉黑好友（状态机 CAS 原子跃迁）
func (s *FriendshipService) BlackFriend(ctx context.Context, userId string, friendId string, expectedVersion ...int64) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能拉黑自己")
	}

	err := WithTx(s.uow, func(tx UoW) error {
		myFs, err := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeForbidden, "你们还不是好友，无法拉黑")
			}
			zap.L().Error("Find friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
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

		ok1, err := tx.FriendshipStore().UpdateStatusWithExpected(ctx, firstUser, secondUser, firstExpected, firstTarget, firstVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected to first error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok1 {
			cur, queryErr := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
			if queryErr != nil || cur == nil {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
			if cur.UserId != userId {
				return errorx.New(errorx.CodeForbidden, "无权操作该好友关系")
			}
			if cur.Status == friendship_status.BE_BLACK {
				return errorx.New(errorx.CodeInvalidParam, "已被对方拉黑，无法重复拉黑")
			}
			return errorx.New(errorx.CodeInvalidParam, "当前状态不允许拉黑或已被处理")
		}

		ok2, err := tx.FriendshipStore().UpdateStatusWithExpected(ctx, secondUser, firstUser, secondExpected, secondTarget, secondVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected to second error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok2 {
			return errorx.New(errorx.CodeInvalidParam, "好友关系状态异常")
		}

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

	return nil
}

// UnblackFriend 取消拉黑好友（状态机 CAS 原子跃迁）
func (s *FriendshipService) UnblackFriend(ctx context.Context, userId string, friendId string, expectedVersion ...int64) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "参数错误")
	}

	err := WithTx(s.uow, func(tx UoW) error {
		myFs, err := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
			zap.L().Error("Find friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
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
			cur, queryErr := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
			if queryErr != nil || cur == nil {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
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

	return nil
}

// UpdateRemark 更新好友备注
func (s *FriendshipService) UpdateRemark(ctx context.Context, userId, friendId, remark string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能给自己设置备注")
	}

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

// GetFriendshipStatus 返回好友关系状态（微信同款对外契约，基于发送方与接收方双向判决）
// 0=非好友(发送方未添加/已删对方) 1=正常好友 2=已拉黑对方(享有单向发信特权) 3=被对方拉黑(拒收) 4=被对方单向删除(朋友验证)
func (s *FriendshipService) GetFriendshipStatus(ctx context.Context, userId, friendId string) (int8, error) {
	senderFs, err := s.uow.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return 0, nil
		}
		zap.L().Error("query sender friendship error", zap.Error(err))
		return 0, errorx.ErrServerBusy
	}

	recipientFs, err := s.uow.FriendshipStore().FindByUserIdAndFriendId(ctx, friendId, userId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return 4, nil
		}
		zap.L().Error("query recipient friendship error", zap.Error(err))
		return 0, errorx.ErrServerBusy
	}

	if recipientFs.Status == friendship_status.BLACK || senderFs.Status == friendship_status.BE_BLACK {
		return 3, nil
	}

	if senderFs.Status == friendship_status.BLACK {
		return 2, nil
	}

	return 1, nil
}

// EstablishFriendship 跨服务建立双向好友关系（供加好友审批通过后同步 RPC 调用）
func (s *FriendshipService) EstablishFriendship(ctx context.Context, userId, friendId string) error {
	if userId == "" || friendId == "" || userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "参数错误")
	}

	err := WithTx(s.uow, func(tx UoW) error {
		newFriendship := Friendship{
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

		anotherFriendship := Friendship{
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

	return nil
}
