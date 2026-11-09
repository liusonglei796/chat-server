package friendship

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"go.uber.org/zap"

	friendshippb "kama_chat_server/api/gen/friendship"
	"kama_chat_server/internal/infra/database"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/errorx"
)

// DeleteFriend 删除好友（微信同款单向删除：仅删除本人与对方的关系记录，对方仍保留单向好友）
func (s *FriendshipService) DeleteFriend(ctx context.Context, userId, friendId string) error {
	if userId == friendId {
		return friendshippb.ErrorCannotOperateSelf("不能删除自己")
	}

	fs, err := s.store.FindByUserIdAndFriendId(ctx, userId, friendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return nil
		}
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return friendshippb.ErrorServerBusy("服务繁忙")
	}
	if fs == nil {
		return nil
	}

	err = database.ExecTx(ctx, s.db, func(txCtx context.Context) error {
		if err := s.store.SoftDelete(txCtx, userId, friendId); err != nil {
			zap.L().Error("Delete friendship error", zap.Error(err))
			return friendshippb.ErrorServerBusy("服务繁忙")
		}

		return s.recordFriendDeletedEvent(txCtx, userId, friendId)
	})

	if err != nil {
		if kerrors.FromError(err).Code != 500 {
			return err
		}
		zap.L().Error("service error", zap.Error(err))
		return friendshippb.ErrorServerBusy("服务繁忙")
	}

	return nil
}

// BlackFriend 拉黑好友（状态机 CAS 原子跃迁）
func (s *FriendshipService) BlackFriend(ctx context.Context, userId string, friendId string, expectedVersion ...int64) error {
	if userId == friendId {
		return friendshippb.ErrorCannotOperateSelf("不能拉黑自己")
	}

	err := database.ExecTx(ctx, s.db, func(txCtx context.Context) error {
		firstUser, secondUser := userId, friendId
		var firstExpected, firstTarget int8 = friendship_status.NORMAL, friendship_status.BLACK
		var secondExpected, secondTarget int8 = friendship_status.NORMAL, friendship_status.BE_BLACK
		var firstExpectedVersion, secondExpectedVersion int64
		if len(expectedVersion) > 0 && expectedVersion[0] > 0 {
			firstExpectedVersion = expectedVersion[0]
		}
		if firstUser > secondUser {
			firstUser, secondUser = friendId, userId
			firstExpected, firstTarget = friendship_status.NORMAL, friendship_status.BE_BLACK
			secondExpected, secondTarget = friendship_status.NORMAL, friendship_status.BLACK
			firstExpectedVersion = 0
			if len(expectedVersion) > 0 && expectedVersion[0] > 0 {
				secondExpectedVersion = expectedVersion[0]
			}
		}

		// 1. 盲写尝试第一顺位更新 (Write-First: 省去前置 SELECT 往返)
		ok1, err := s.store.UpdateStatusWithExpected(txCtx, firstUser, secondUser, firstExpected, firstTarget, firstExpectedVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected to first error", zap.Error(err))
			return friendshippb.ErrorServerBusy("服务繁忙")
		}
		if !ok1 {
			// 失败回捞 (Read-on-Failure)：查明具体原因以返回精准业务错误码
			cur, queryErr := s.store.FindByUserIdAndFriendId(txCtx, userId, friendId)
			if queryErr != nil || cur == nil {
				return friendshippb.ErrorNotFriends("你们还不是好友，无法拉黑")
			}
			if len(expectedVersion) > 0 && expectedVersion[0] > 0 && cur.Version != expectedVersion[0] {
				return friendshippb.ErrorFriendshipConflict("好友状态已被并发修改或请求已过期")
			}
			if cur.Status == friendship_status.BE_BLACK {
				return friendshippb.ErrorAlreadyBlocked("已被对方拉黑，无法重复拉黑")
			}
			return friendshippb.ErrorInvalidParam("当前状态不允许拉黑")
		}

		// 2. 盲写尝试第二顺位更新 (Write-First)
		ok2, err := s.store.UpdateStatusWithExpected(txCtx, secondUser, firstUser, secondExpected, secondTarget, secondExpectedVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected to second error", zap.Error(err))
			return friendshippb.ErrorServerBusy("服务繁忙")
		}
		if !ok2 {
			theirFs, queryErr := s.store.FindByUserIdAndFriendId(txCtx, friendId, userId)
			if queryErr != nil || theirFs == nil {
				return friendshippb.ErrorFriendNotFound("对方好友关系不存在")
			}
			return friendshippb.ErrorInvalidParam("好友关系状态异常")
		}

		return s.recordFriendBlackedEvent(txCtx, userId, friendId)
	})

	if err != nil {
		if kerrors.FromError(err).Code != 500 {
			return err
		}
		zap.L().Error("BlackFriend transaction error", zap.Error(err))
		return friendshippb.ErrorServerBusy("服务繁忙")
	}

	return nil
}

// UnblackFriend 取消拉黑好友（状态机 CAS 原子跃迁）
func (s *FriendshipService) UnblackFriend(ctx context.Context, userId string, friendId string, expectedVersion ...int64) error {
	if userId == friendId {
		return friendshippb.ErrorCannotOperateSelf("参数错误")
	}

	err := database.ExecTx(ctx, s.db, func(txCtx context.Context) error {
		firstUser, secondUser := userId, friendId
		var firstExpected, firstTarget int8 = friendship_status.BLACK, friendship_status.NORMAL
		var secondExpected, secondTarget int8 = friendship_status.BE_BLACK, friendship_status.NORMAL
		var firstExpectedVersion, secondExpectedVersion int64
		if len(expectedVersion) > 0 && expectedVersion[0] > 0 {
			firstExpectedVersion = expectedVersion[0]
		}
		if firstUser > secondUser {
			firstUser, secondUser = friendId, userId
			firstExpected, firstTarget = friendship_status.BE_BLACK, friendship_status.NORMAL
			secondExpected, secondTarget = friendship_status.BLACK, friendship_status.NORMAL
			firstExpectedVersion = 0
			if len(expectedVersion) > 0 && expectedVersion[0] > 0 {
				secondExpectedVersion = expectedVersion[0]
			}
		}

		// 1. 盲写尝试第一顺位更新 (Write-First: 省去前置 SELECT 往返)
		ok1, err := s.store.UpdateStatusWithExpected(txCtx, firstUser, secondUser, firstExpected, firstTarget, firstExpectedVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected first error", zap.Error(err))
			return friendshippb.ErrorServerBusy("服务繁忙")
		}
		if !ok1 {
			// 失败回捞 (Read-on-Failure)
			cur, queryErr := s.store.FindByUserIdAndFriendId(txCtx, userId, friendId)
			if queryErr != nil || cur == nil {
				return friendshippb.ErrorFriendNotFound("好友关系不存在")
			}
			if len(expectedVersion) > 0 && expectedVersion[0] > 0 && cur.Version != expectedVersion[0] {
				return friendshippb.ErrorFriendshipConflict("好友状态已被并发修改或请求已过期")
			}
			if cur.Status != friendship_status.BLACK {
				return friendshippb.ErrorNotBlocked("未拉黑该好友，无需解除拉黑")
			}
			return friendshippb.ErrorInvalidParam("好友关系状态已变更，无需重复操作")
		}

		// 2. 盲写尝试第二顺位更新 (Write-First)
		ok2, err := s.store.UpdateStatusWithExpected(txCtx, secondUser, firstUser, secondExpected, secondTarget, secondExpectedVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected second error", zap.Error(err))
			return friendshippb.ErrorServerBusy("服务繁忙")
		}
		if !ok2 {
			theirFs, queryErr := s.store.FindByUserIdAndFriendId(txCtx, friendId, userId)
			if queryErr != nil || theirFs == nil {
				return friendshippb.ErrorFriendNotFound("对方好友关系不存在")
			}
			return friendshippb.ErrorInvalidParam("数据状态异常，请联系管理员")
		}

		return nil
	})

	if err != nil {
		if kerrors.FromError(err).Code != 500 {
			return err
		}
		zap.L().Error("UnblackFriend transaction error", zap.Error(err))
		return friendshippb.ErrorServerBusy("服务繁忙")
	}

	return nil
}

// UpdateRemark 更新好友备注
func (s *FriendshipService) UpdateRemark(ctx context.Context, userId, friendId, remark string) error {
	if userId == friendId {
		return friendshippb.ErrorCannotOperateSelf("不能给自己设置备注")
	}

	isFriend, err := s.store.IsFriend(ctx, userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return friendshippb.ErrorServerBusy("服务繁忙")
	}
	if !isFriend {
		return friendshippb.ErrorNotFriends("你们还不是好友")
	}

	if err := s.store.UpdateRemark(ctx, userId, friendId, remark); err != nil {
		zap.L().Error("Update remark error",
			zap.String("user_id", userId),
			zap.String("friend_id", friendId),
			zap.Error(err),
		)
		return friendshippb.ErrorServerBusy("服务繁忙")
	}

	return nil
}

// EstablishFriendship 跨服务建立双向好友关系（供加好友审批通过后同步 RPC 调用）
func (s *FriendshipService) EstablishFriendship(ctx context.Context, userId, friendId string) error {
	if userId == "" || friendId == "" || userId == friendId {
		return friendshippb.ErrorInvalidParam("参数错误")
	}

	// 固定按主键排序顺序加锁和插入，彻底杜绝双向并发死锁
	u1, u2 := userId, friendId
	if u1 > u2 {
		u1, u2 = u2, u1
	}

	err := database.ExecTx(ctx, s.db, func(txCtx context.Context) error {
		firstFriendship := Friendship{
			UserId:   u1,
			FriendId: u2,
			Status:   friendship_status.NORMAL,
			Version:  1,
		}
		if err := s.store.UpsertFriendship(txCtx, &firstFriendship); err != nil {
			return err
		}

		secondFriendship := Friendship{
			UserId:   u2,
			FriendId: u1,
			Status:   friendship_status.NORMAL,
			Version:  1,
		}
		if err := s.store.UpsertFriendship(txCtx, &secondFriendship); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		zap.L().Error("EstablishFriendship transaction failed",
			zap.String("userId", userId),
			zap.String("friendId", friendId),
			zap.Error(err),
		)
		return friendshippb.ErrorServerBusy("服务繁忙")
	}

	return nil
}
