package user

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authpb "kama_chat_server/api/gen/auth"
	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
)

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

// extractClientIP 从传输上下文获取客户端真实 IP
func extractClientIP(ctx context.Context) string {
	if tr, ok := transport.FromServerContext(ctx); ok {
		ip := tr.RequestHeader().Get("X-Forwarded-For")
		if ip != "" {
			return ip
		}
		return tr.RequestHeader().Get("X-Real-IP")
	}
	return ""
}

// ==================== UserGrpcServer ====================

type UserGrpcServer struct {
	userpb.UnimplementedUserServiceServer
	svc *UserService
}

func NewUserGrpcServer(svc *UserService) *UserGrpcServer {
	return &UserGrpcServer{svc: svc}
}

func (s *UserGrpcServer) UpdateUserInfo(ctx context.Context, req *userpb.UpdateUserInfoRequest) (*userpb.UpdateUserInfoResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	updateReq := UpdateUserInfoRequest{
		Email:     req.Email,
		Nickname:  req.Nickname,
		Birthday:  req.Birthday,
		Signature: req.Signature,
		Avatar:    req.Avatar,
	}

	err := s.svc.UpdateUserInfo(ctx, userId, updateReq)
	if err != nil {
		return nil, err
	}
	return &userpb.UpdateUserInfoResponse{}, nil
}

func (s *UserGrpcServer) GetUserInfo(ctx context.Context, req *userpb.GetUserInfoRequest) (*userpb.GetUserInfoResponse, error) {
	requesterId := req.RequesterId
	if requesterId == "" {
		requesterId = extractTrustedUserID(ctx)
	}
	if requesterId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	targetId := req.TargetId
	if targetId == "" {
		targetId = requesterId
	}

	info, err := s.svc.GetUserInfo(ctx, requesterId, targetId)
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

func (s *UserGrpcServer) GetPublicUserInfo(ctx context.Context, req *userpb.GetPublicUserInfoRequest) (*userpb.GetPublicUserInfoResponse, error) {
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

func (s *UserGrpcServer) BatchGetPublicUserInfo(ctx context.Context, req *userpb.BatchGetPublicUserInfoRequest) (*userpb.BatchGetPublicUserInfoResponse, error) {
	infoList, err := s.svc.BatchGetPublicUserInfo(ctx, req.UserIds)
	if err != nil {
		return nil, err
	}
	users := make([]*userpb.PublicUserInfo, 0, len(infoList))
	for _, info := range infoList {
		users = append(users, &userpb.PublicUserInfo{
			Uuid:      info.Uuid,
			Nickname:  info.Nickname,
			Avatar:    info.Avatar,
			Gender:    int32(info.Gender),
			Birthday:  info.Birthday,
			Signature: info.Signature,
		})
	}
	return &userpb.BatchGetPublicUserInfoResponse{Users: users}, nil
}

func (s *UserGrpcServer) GetUserStatus(ctx context.Context, req *userpb.GetUserStatusRequest) (*userpb.GetUserStatusResponse, error) {
	userStatus, err := s.svc.GetUserStatus(ctx, req.UserId)
	if err != nil {
		var codeErr *errorx.CodeError
		if errors.As(err, &codeErr) && codeErr.Code == errorx.CodeUserNotExist {
			return nil, status.Error(codes.NotFound, codeErr.Msg)
		}
		return nil, err
	}
	return &userpb.GetUserStatusResponse{Status: int32(userStatus)}, nil
}

func (s *UserGrpcServer) KickUser(ctx context.Context, req *userpb.KickUserRequest) (*userpb.KickUserResponse, error) {
	err := s.svc.KickUser(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return &userpb.KickUserResponse{}, nil
}

// ==================== AuthGrpcServer ====================

type AuthGrpcServer struct {
	authpb.UnimplementedAuthServiceServer
	svc     *AuthService
	userSvc *UserService
}

func NewAuthGrpcServer(svc *AuthService, userSvc *UserService) *AuthGrpcServer {
	return &AuthGrpcServer{svc: svc, userSvc: userSvc}
}

func (s *AuthGrpcServer) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	clientIp := req.ClientIp
	if clientIp == "" {
		clientIp = extractClientIP(ctx)
	}

	loginReq := LoginRequest{
		Username: req.Username,
		Password: req.Password,
	}

	rsp, err := s.userSvc.Login(ctx, loginReq, clientIp)
	if err != nil {
		return nil, err
	}

	return buildLoginResponse(rsp), nil
}

func (s *AuthGrpcServer) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	clientIp := req.ClientIp
	if clientIp == "" {
		clientIp = extractClientIP(ctx)
	}

	clientToken := req.ClientToken
	if clientToken == "" {
		clientToken = interceptor.GetIdempotencyKeyFromContext(ctx)
	}

	regReq := RegisterRequest{
		Username:    req.Username,
		Password:    req.Password,
		ClientToken: clientToken,
	}

	rsp, err := s.userSvc.Register(ctx, regReq, clientIp)
	if err != nil {
		return nil, err
	}

	return buildRegisterResponse(rsp), nil
}

func (s *AuthGrpcServer) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.userSvc.Logout(ctx, userId)
	if err != nil {
		return nil, err
	}
	return &authpb.LogoutResponse{}, nil
}

func (s *AuthGrpcServer) ValidateTokenID(ctx context.Context, req *authpb.ValidateTokenIDRequest) (*authpb.ValidateTokenIDResponse, error) {
	valid, err := s.svc.ValidateTokenID(ctx, req.UserId, req.TokenId)
	if err != nil {
		return nil, err
	}
	return &authpb.ValidateTokenIDResponse{IsValid: valid}, nil
}

func (s *AuthGrpcServer) GetUserIsAdmin(ctx context.Context, req *authpb.GetUserIsAdminRequest) (*authpb.GetUserIsAdminResponse, error) {
	isAdmin, err := s.svc.GetUserIsAdmin(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return &authpb.GetUserIsAdminResponse{IsAdmin: isAdmin}, nil
}

func buildLoginResponse(rsp *LoginRespond) *authpb.LoginResponse {
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

func buildRegisterResponse(rsp *RegisterRespond) *authpb.RegisterResponse {
	return &authpb.RegisterResponse{
		Uuid:      rsp.Uuid,
		Telephone: rsp.Telephone,
		Nickname:  rsp.Nickname,
		Email:     rsp.Email,
		Avatar:    rsp.Avatar,
		Gender:    int32(rsp.Gender),
		Birthday:  rsp.Birthday,
		Signature: rsp.Signature,
		IsAdmin:   int32(rsp.IsAdmin),
		Status:    int32(rsp.Status),
		CreatedAt: rsp.CreatedAt,
	}
}
