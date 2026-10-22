package friendship

import (
	"context"

	"github.com/go-kratos/kratos/v2/transport"

	friendshippb "kama_chat_server/api/gen/friendship"
	"kama_chat_server/pkg/errorx"
)

type GrpcServer struct {
	friendshippb.UnimplementedFriendshipServiceServer
	friendshipSvc *FriendshipService
}

func NewGrpcServer(friendshipSvc *FriendshipService) *GrpcServer {
	return &GrpcServer{friendshipSvc: friendshipSvc}
}

// extractTrustedUserID 从 Kratos 传输上下文或 Context 中提取网关注入的受信用户身份
func extractTrustedUserID(ctx context.Context) string {
	if tr, ok := transport.FromServerContext(ctx); ok {
		uid := tr.RequestHeader().Get("X-User-Id")
		if uid != "" {
			return uid
		}
	}
	if val := ctx.Value("user_id"); val != nil {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func (s *GrpcServer) GetFriendList(ctx context.Context, req *friendshippb.GetFriendListRequest) (*friendshippb.GetFriendListResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	page := int(req.Page)
	if page < 1 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize < 1 {
		pageSize = 20
	}

	list, total, err := s.friendshipSvc.GetFriendList(ctx, userId, page, pageSize)
	if err != nil {
		return nil, err
	}

	var rspList []*friendshippb.MyUserListRespond
	for _, v := range list {
		rspList = append(rspList, &friendshippb.MyUserListRespond{
			UserId:   v.UserId,
			UserName: v.UserName,
			Avatar:   v.Avatar,
		})
	}
	return &friendshippb.GetFriendListResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) GetFriendInfo(ctx context.Context, req *friendshippb.GetFriendInfoRequest) (*friendshippb.GetFriendInfoResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	info, err := s.friendshipSvc.GetFriendInfo(ctx, userId, req.FriendId)
	if err != nil {
		return nil, err
	}

	return &friendshippb.GetFriendInfoResponse{
		FriendId:        info.FriendId,
		FriendName:      info.FriendName,
		FriendAvatar:    info.FriendAvatar,
		FriendBirthday:  info.FriendBirthday,
		FriendEmail:     info.FriendEmail,
		FriendPhone:     info.FriendPhone,
		FriendGender:    int32(info.FriendGender),
		FriendSignature: info.FriendSignature,
		Remark:          info.Remark,
	}, nil
}

func (s *GrpcServer) DeleteFriend(ctx context.Context, req *friendshippb.DeleteFriendRequest) (*friendshippb.DeleteFriendResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.friendshipSvc.DeleteFriend(ctx, userId, req.FriendId)
	return &friendshippb.DeleteFriendResponse{}, err
}

func (s *GrpcServer) BlackFriend(ctx context.Context, req *friendshippb.BlackFriendRequest) (*friendshippb.BlackFriendResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.friendshipSvc.BlackFriend(ctx, userId, req.FriendId)
	return &friendshippb.BlackFriendResponse{}, err
}

func (s *GrpcServer) UnblackFriend(ctx context.Context, req *friendshippb.UnblackFriendRequest) (*friendshippb.UnblackFriendResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.friendshipSvc.UnblackFriend(ctx, userId, req.FriendId)
	return &friendshippb.UnblackFriendResponse{}, err
}

func (s *GrpcServer) UpdateRemark(ctx context.Context, req *friendshippb.UpdateRemarkRequest) (*friendshippb.UpdateRemarkResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.friendshipSvc.UpdateRemark(ctx, userId, req.FriendId, req.Remark)
	return &friendshippb.UpdateRemarkResponse{}, err
}

func (s *GrpcServer) CheckFriendship(ctx context.Context, req *friendshippb.CheckFriendshipRequest) (*friendshippb.CheckFriendshipResponse, error) {
	fs, err := s.friendshipSvc.GetFriendshipStatus(ctx, req.UserId, req.FriendId)
	if err != nil {
		return nil, err
	}
	return &friendshippb.CheckFriendshipResponse{Status: int32(fs)}, nil
}

func (s *GrpcServer) EstablishFriendship(ctx context.Context, req *friendshippb.EstablishFriendshipRequest) (*friendshippb.EstablishFriendshipResponse, error) {
	err := s.friendshipSvc.EstablishFriendship(ctx, req.UserId, req.FriendId)
	return &friendshippb.EstablishFriendshipResponse{}, err
}
