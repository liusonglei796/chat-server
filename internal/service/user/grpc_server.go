package user

import (
	"context"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/dto/request/user"
)

type GrpcServer struct {
	userpb.UnimplementedUserServiceServer
	svc *UserService
}

func NewGrpcServer(svc *UserService) *GrpcServer {
	return &GrpcServer{svc: svc}
}

func (s *GrpcServer) UpdateUserInfo(ctx context.Context, req *userpb.UpdateUserInfoRequest) (*userpb.UpdateUserInfoResponse, error) {
	updateReq := user.UpdateUserInfoRequest{
		Email:     req.Email,
		Nickname:  req.Nickname,
		Birthday:  req.Birthday,
		Signature: req.Signature,
		Avatar:    req.Avatar,
	}

	err := s.svc.UpdateUserInfo(ctx, req.UserId, updateReq)
	if err != nil {
		return nil, err
	}
	return &userpb.UpdateUserInfoResponse{}, nil
}

func (s *GrpcServer) GetUserInfo(ctx context.Context, req *userpb.GetUserInfoRequest) (*userpb.GetUserInfoResponse, error) {
	info, err := s.svc.GetUserInfo(ctx, req.RequesterId, req.TargetId)
	if err != nil {
		return nil, err
	}
	return &userpb.GetUserInfoResponse{
		Uuid:      info.Uuid,
		Telephone: info.Telephone,
		Nickname:  info.Nickname,
		Avatar:    info.Avatar,
		Birthday:  info.Birthday,
		Email:     info.Email,
		Gender:    int32(info.Gender),
		Signature: info.Signature,
		CreatedAt: info.CreatedAt,
		IsAdmin:   int32(info.IsAdmin),
		Status:    int32(info.Status),
	}, nil
}

func (s *GrpcServer) GetPublicUserInfo(ctx context.Context, req *userpb.GetPublicUserInfoRequest) (*userpb.GetPublicUserInfoResponse, error) {
	info, err := s.svc.GetPublicUserInfo(ctx, req.TargetId)
	if err != nil {
		return nil, err
	}
	return &userpb.GetPublicUserInfoResponse{
		Uuid:      info.Uuid,
		Nickname:  info.Nickname,
		Avatar:    info.Avatar,
		Gender:    int32(info.Gender),
		Birthday:  info.Birthday,
		Signature: info.Signature,
	}, nil
}

func (s *GrpcServer) KickUser(ctx context.Context, req *userpb.KickUserRequest) (*userpb.KickUserResponse, error) {
	err := s.svc.KickUser(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return &userpb.KickUserResponse{}, nil
}
