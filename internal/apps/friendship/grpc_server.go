package friendship

import (
	"context"

	friendshippb "kama_chat_server/api/gen/friendship"
)

type GrpcServer struct {
	friendshippb.UnimplementedFriendshipServiceServer
	friendshipSvc *FriendshipService
}

func NewGrpcServer(friendshipSvc *FriendshipService) *GrpcServer {
	return &GrpcServer{friendshipSvc: friendshipSvc}
}

func (s *GrpcServer) GetFriendList(ctx context.Context, req *friendshippb.GetFriendListRequest) (*friendshippb.GetFriendListResponse, error) {
	list, total, err := s.friendshipSvc.GetFriendList(ctx, req.UserId, int(req.Page), int(req.PageSize))
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
	info, err := s.friendshipSvc.GetFriendInfo(ctx, req.UserId, req.FriendId)
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
	err := s.friendshipSvc.DeleteFriend(ctx, req.UserId, req.FriendId)
	return &friendshippb.DeleteFriendResponse{}, err
}

func (s *GrpcServer) BlackFriend(ctx context.Context, req *friendshippb.BlackFriendRequest) (*friendshippb.BlackFriendResponse, error) {
	err := s.friendshipSvc.BlackFriend(ctx, req.UserId, req.FriendId)
	return &friendshippb.BlackFriendResponse{}, err
}

func (s *GrpcServer) UnblackFriend(ctx context.Context, req *friendshippb.UnblackFriendRequest) (*friendshippb.UnblackFriendResponse, error) {
	err := s.friendshipSvc.UnblackFriend(ctx, req.UserId, req.FriendId)
	return &friendshippb.UnblackFriendResponse{}, err
}

func (s *GrpcServer) UpdateRemark(ctx context.Context, req *friendshippb.UpdateRemarkRequest) (*friendshippb.UpdateRemarkResponse, error) {
	err := s.friendshipSvc.UpdateRemark(ctx, req.UserId, req.FriendId, req.Remark)
	return &friendshippb.UpdateRemarkResponse{}, err
}

func (s *GrpcServer) CheckFriendship(ctx context.Context, req *friendshippb.CheckFriendshipRequest) (*friendshippb.CheckFriendshipResponse, error) {
	fs, err := s.friendshipSvc.GetFriendshipStatus(ctx, req.UserId, req.FriendId)
	if err != nil {
		return nil, err
	}
	return &friendshippb.CheckFriendshipResponse{Status: int32(fs)}, nil
}
