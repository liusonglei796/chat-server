package auth

import (
	"context"

	authpb "kama_chat_server/api/gen/auth"
	userrsp "kama_chat_server/internal/common/dto/respond/user"
	"kama_chat_server/internal/apps/user/user"
	"kama_chat_server/internal/common/dto/request/auth"
)

type GrpcServer struct {
	authpb.UnimplementedAuthServiceServer
	svc     *Service
	userSvc *user.UserService
}

func NewGrpcServer(svc *Service, userSvc *user.UserService) *GrpcServer {
	return &GrpcServer{svc: svc, userSvc: userSvc}
}

func (s *GrpcServer) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	loginReq := auth.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	}
	rsp, err := s.userSvc.Login(ctx, loginReq, req.ClientIp)
	if err != nil {
		return nil, err
	}
	return buildLoginResponse(rsp), nil
}

func (s *GrpcServer) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	regReq := auth.RegisterRequest{
		Username: req.Username,
		Password: req.Password,
	}
	rsp, err := s.userSvc.Register(ctx, regReq, req.ClientIp)
	if err != nil {
		return nil, err
	}
	return buildRegisterResponse(rsp), nil
}

func (s *GrpcServer) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	err := s.userSvc.Logout(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return &authpb.LogoutResponse{}, nil
}

func (s *GrpcServer) ValidateTokenID(ctx context.Context, req *authpb.ValidateTokenIDRequest) (*authpb.ValidateTokenIDResponse, error) {
	valid, err := s.svc.ValidateTokenID(ctx, req.UserId, req.TokenId)
	if err != nil {
		return nil, err
	}
	return &authpb.ValidateTokenIDResponse{IsValid: valid}, nil
}

func (s *GrpcServer) GetUserIsAdmin(ctx context.Context, req *authpb.GetUserIsAdminRequest) (*authpb.GetUserIsAdminResponse, error) {
	isAdmin, err := s.svc.GetUserIsAdmin(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return &authpb.GetUserIsAdminResponse{IsAdmin: isAdmin}, nil
}

func buildLoginResponse(rsp *userrsp.LoginRespond) *authpb.LoginResponse {
	return &authpb.LoginResponse{
		Uuid:         rsp.Uuid,
		Telephone:    rsp.Telephone,
		Nickname:     rsp.Nickname,
		Email:        rsp.Email,
		Avatar:       rsp.Avatar,
		Gender:       int32(rsp.Gender),
		Birthday:     rsp.Birthday,
		Signature:    rsp.Signature,
		IsAdmin:      int32(rsp.IsAdmin),
		Status:       int32(rsp.Status),
		AccessToken:  rsp.AccessToken,
		RefreshToken: rsp.RefreshToken,
		CreatedAt:    rsp.CreatedAt,
	}
}

func buildRegisterResponse(rsp *userrsp.LoginRespond) *authpb.RegisterResponse {
	return &authpb.RegisterResponse{
		Uuid:         rsp.Uuid,
		Telephone:    rsp.Telephone,
		Nickname:     rsp.Nickname,
		Email:        rsp.Email,
		Avatar:       rsp.Avatar,
		Gender:       int32(rsp.Gender),
		Birthday:     rsp.Birthday,
		Signature:    rsp.Signature,
		IsAdmin:      int32(rsp.IsAdmin),
		Status:       int32(rsp.Status),
		AccessToken:  rsp.AccessToken,
		RefreshToken: rsp.RefreshToken,
		CreatedAt:    rsp.CreatedAt,
	}
}
