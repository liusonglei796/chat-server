package friendship

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/dto/event"
	friendshiprsp "kama_chat_server/internal/common/dto/respond/friendship"
	userrsp "kama_chat_server/internal/common/dto/respond/user"
	"kama_chat_server/internal/common/grpc_client"
	"kama_chat_server/pkg/constants"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
)

// friendshipUoW 好友关系服务的 UoW 接口：事务/事件能力 + 仅本服务拥有的 Friendship Store
type friendshipUoW interface {
	store.TxExecutor
	RecordEvent(ctx context.Context, eventType string, payload []byte) error
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
		return errorx.New(errorx.CodeForbidden, "你们还不是好友")
	}

	err = store.WithTx(s.uow, func(tx friendshipUoW) error {
		if err := tx.FriendshipStore().SoftDelete(ctx, userId, friendId); err != nil {
			zap.L().Error("Delete friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		return nil
	})

	if err != nil {
		zap.L().Error("service error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	s.clearFriendRelationCache(userId, friendId)

	return nil
}

// BlackFriend 拉黑好友
func (s *FriendshipService) BlackFriend(ctx context.Context, userId string, friendId string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "不能拉黑自己")
	}

	err := store.WithTx(s.uow, func(tx friendshipUoW) error {
		myFs, err := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeForbidden, "你们还不是好友，无法拉黑")
			}
			zap.L().Error("Find friendship error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if myFs.Status != friendship_status.NORMAL {
			return errorx.New(errorx.CodeInvalidParam, "当前状态不允许拉黑")
		}

		if err := tx.FriendshipStore().UpdateStatus(ctx, userId, friendId, friendship_status.BLACK); err != nil {
			zap.L().Error("Update status to BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		if err := tx.FriendshipStore().UpdateStatus(ctx, friendId, userId, friendship_status.BE_BLACK); err != nil {
			zap.L().Error("Update status to BE_BLACK error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		// 写 outbox：message_service 消费后软删双方私聊会话（session 表归 message 库，跨库写走事件）
		payload, _ := json.Marshal(event.FriendBlackedEvent{UserId: userId, FriendId: friendId})
		if err := tx.RecordEvent(ctx, event.EventFriendBlacked, payload); err != nil {
			zap.L().Error("Record friend blacked event error", zap.Error(err))
			return errorx.ErrServerBusy
		}

		return nil
	})

	if err != nil {
		if e, ok := err.(*errorx.CodeError); ok {
			return e
		}
		zap.L().Error("BlackFriend transaction error", zap.Error(err))
		return errorx.ErrServerBusy
	}

	s.clearFriendRelationCache(userId, friendId)

	return nil
}

// UnblackFriend 取消拉黑好友
func (s *FriendshipService) UnblackFriend(ctx context.Context, userId string, friendId string) error {
	if userId == friendId {
		return errorx.New(errorx.CodeInvalidParam, "参数错误")
	}

	err := store.WithTx(s.uow, func(tx friendshipUoW) error {
		myFs, err := tx.FriendshipStore().FindByUserIdAndFriendId(ctx, userId, friendId)
		if err != nil {
			if errorx.IsNotFound(err) {
				return errorx.New(errorx.CodeNotFound, "好友关系不存在")
			}
			zap.L().Error("Find friendship error", zap.Error(err))
			return errorx.ErrServerBusy
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

		if err := tx.FriendshipStore().UpdateStatus(ctx, userId, friendId, friendship_status.NORMAL); err != nil {
			zap.L().Error("Update friendship status error", zap.Error(err))
			return errorx.ErrServerBusy
		}
		if err := tx.FriendshipStore().UpdateStatus(ctx, friendId, userId, friendship_status.NORMAL); err != nil {
			zap.L().Error("Update reverse friendship status error", zap.Error(err))
			return errorx.ErrServerBusy
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
