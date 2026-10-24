package friendship

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/pkg/enum/friendship/friendship_status"
	"kama_chat_server/pkg/enum/user/user_status"
	"kama_chat_server/pkg/errorx"
)

// GetFriendList 获取好友列表（分页）
func (s *FriendshipService) GetFriendList(ctx context.Context, userId string, page, pageSize int) ([]FriendItem, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	friendships, total, err := s.store.FindFriendsByUserId(ctx, userId, page, pageSize)
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

	isFriend, err := s.store.IsFriend(ctx, userId, friendId)
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
	fs, err := s.store.FindByUserIdAndFriendId(ctx, userId, friendId)
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

// GetFriendshipStatus 返回好友关系状态（微信同款对外契约，基于发送方与接收方双向判决）
// 0=非好友(发送方未添加/已删对方) 1=正常好友 2=已拉黑对方(享有单向发信特权) 3=被对方拉黑(拒收) 4=被对方单向删除(朋友验证)
func (s *FriendshipService) GetFriendshipStatus(ctx context.Context, userId, friendId string) (int8, error) {
	senderFs, err := s.store.FindByUserIdAndFriendId(ctx, userId, friendId)
	if err != nil {
		if errorx.IsNotFound(err) {
			return 0, nil
		}
		zap.L().Error("query sender friendship error", zap.Error(err))
		return 0, errorx.ErrServerBusy
	}

	recipientFs, err := s.store.FindByUserIdAndFriendId(ctx, friendId, userId)
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
