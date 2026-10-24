package friendship

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	"kama_chat_server/internal/infra/dberr"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/event"
)

// DeleteFriend 删除好友（微信同款单向删除：仅删除本人与对方的关系记录，对方仍保留单向好友）
func (s *FriendshipService) DeleteFriend(ctx context.Context, userId, friendId string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能删除自己")
	}

	fs, err := s.store.FindByUserIdAndFriendId(ctx, userId, friendId)
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

	err = s.execTx(ctx, func(txStore Store, recordEvent func(aggregateId, eventType string, payload []byte) error) error {
		if err := txStore.SoftDelete(ctx, userId, friendId); err != nil {
			zap.L().Error("Delete friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		payload, _ := json.Marshal(event.FriendDeletedEvent{
			UserId:   userId,
			FriendId: friendId,
		})
		return recordEvent(userId, event.EventFriendDeleted, payload)
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

	err := s.execTx(ctx, func(txStore Store, recordEvent func(aggregateId, eventType string, payload []byte) error) error {
		myFs, err := txStore.FindByUserIdAndFriendId(ctx, userId, friendId)
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

		theirFs, err := txStore.FindByUserIdAndFriendId(ctx, friendId, userId)
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

		ok1, err := txStore.UpdateStatusWithExpected(ctx, firstUser, secondUser, firstExpected, firstTarget, firstVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected to first error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok1 {
			cur, queryErr := txStore.FindByUserIdAndFriendId(ctx, userId, friendId)
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

		ok2, err := txStore.UpdateStatusWithExpected(ctx, secondUser, firstUser, secondExpected, secondTarget, secondVersion)
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
		return recordEvent(userId, event.EventFriendBlacked, payload)
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

	err := s.execTx(ctx, func(txStore Store, recordEvent func(aggregateId, eventType string, payload []byte) error) error {
		myFs, err := txStore.FindByUserIdAndFriendId(ctx, userId, friendId)
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

		theirFs, err := txStore.FindByUserIdAndFriendId(ctx, friendId, userId)
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

		ok1, err := txStore.UpdateStatusWithExpected(ctx, firstUser, secondUser, firstExpected, firstTarget, firstVersion)
		if err != nil {
			zap.L().Error("UpdateStatusWithExpected first error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if !ok1 {
			cur, queryErr := txStore.FindByUserIdAndFriendId(ctx, userId, friendId)
			if queryErr != nil || cur == nil {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
			if cur.UserId != userId {
				return errorx.New(errorx.CodeForbidden, "无权操作该好友关系")
			}
			return errorx.New(errorx.CodeInvalidParam, "好友关系状态已变更，无需重复操作")
		}

		ok2, err := txStore.UpdateStatusWithExpected(ctx, secondUser, firstUser, secondExpected, secondTarget, secondVersion)
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

	isFriend, err := s.store.IsFriend(ctx, userId, friendId)
	if err != nil {
		zap.L().Error("Check friend relationship error", zap.Error(err))
		return errorx.ErrServerBusy
	}
	if !isFriend {
		return errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	if err := s.store.UpdateRemark(ctx, userId, friendId, remark); err != nil {
		zap.L().Error("Update remark error",
			zap.String("user_id", userId),
			zap.String("friend_id", friendId),
			zap.Error(err),
		)
		return errorx.ErrServerBusy
	}

	return nil
}

// EstablishFriendship 跨服务建立双向好友关系（供加好友审批通过后同步 RPC 调用）
func (s *FriendshipService) EstablishFriendship(ctx context.Context, userId, friendId string) error {
	if userId == "" || friendId == "" || userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "参数错误")
	}

	err := s.execTx(ctx, func(txStore Store, recordEvent func(aggregateId, eventType string, payload []byte) error) error {
		newFriendship := Friendship{
			UserId:   userId,
			FriendId: friendId,
			Status:   friendship_status.NORMAL,
			Version:  1,
		}
		if err := txStore.CreateFriendship(ctx, &newFriendship); err != nil {
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
		if err := txStore.CreateFriendship(ctx, &anotherFriendship); err != nil {
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
