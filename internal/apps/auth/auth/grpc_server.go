package auth

import (
	"context"

	"github.com/go-kratos/kratos/v2/transport"

	authpb "kama_chat_server/api/gen/auth"
	"kama_chat_server/internal/apps/user/user"
	"kama_chat_server/internal/common/dto/request/auth"
	userrsp "kama_chat_server/internal/common/dto/respond/user"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
)

type GrpcServer struct {
	authpb.UnimplementedAuthServiceServer
	svc     *Service
	userSvc *user.UserService
}

func NewGrpcServer(svc *Service, userSvc *user.UserService) *GrpcServer {
	return &GrpcServer{svc: svc, userSvc: userSvc}
}

// extractClientIP 从传输上下文获取客户端真实 IP
// 来源: Kratos HTTP transport 请求头 X-Forwarded-For 或 X-Real-IP
// 用于: 登录与注册风控审计入参
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

// extractTrustedUserID 从传输上下文获取网关注入的受信用户身份
// 来源: Kratos HTTP transport 请求头 X-User-Id
// 用于: 登出操作身份确认
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

func (s *GrpcServer) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	// Step 1.1: 组装登录参数并推导客户端网络地址
	// 来源: req.Username, req.Password 及 HTTP 请求头
	// 用于: Step 1.2 调用领域层验证用户凭证
	clientIp := req.ClientIp
	if clientIp == "" {
		clientIp = extractClientIP(ctx)
	}

	loginReq := auth.LoginRequest{
		Username: req.Username,
		Password: req.Password,
	}

	// Step 1.2: 执行领域服务登录逻辑
	// 来源: Step 1.1 组装的 loginReq 与 clientIp
	// 用于: Step 1.3 转换输出登录响应协议
	rsp, err := s.userSvc.Login(ctx, loginReq, clientIp)
	if err != nil {
		return nil, err
	}

	// Step 1.3: 封装输出标准登录协议数据
	// 来源: Step 1.2 返回的用户登录结果 DTO
	// 用于: 返回给网关或调用端
	return buildLoginResponse(rsp), nil
}

func (s *GrpcServer) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	// Step 2.1: 组装注册请求参数
	// 来源: req.Username, req.Password 及 HTTP 请求头
	// 用于: Step 2.2 调用领域层注册新用户
	clientIp := req.ClientIp
	if clientIp == "" {
		clientIp = extractClientIP(ctx)
	}

	clientToken := req.ClientToken
	if clientToken == "" {
		clientToken = interceptor.GetIdempotencyKeyFromContext(ctx)
	}

	regReq := auth.RegisterRequest{
		Username:    req.Username,
		Password:    req.Password,
		ClientToken: clientToken,
	}

	// Step 2.2: 执行领域服务注册逻辑
	// 来源: Step 2.1 组装的 regReq 与 clientIp
	// 用于: Step 2.3 转换输出注册响应协议
	rsp, err := s.userSvc.Register(ctx, regReq, clientIp)
	if err != nil {
		return nil, err
	}

	// Step 2.3: 封装输出标准注册协议数据
	// 来源: Step 2.2 返回的新建用户 DTO
	// 用于: 返回给网关或调用端
	return buildRegisterResponse(rsp), nil
}

func (s *GrpcServer) Logout(ctx context.Context, req *authpb.LogoutRequest) (*authpb.LogoutResponse, error) {
	// Step 3.1: 确认登出操作用户身份
	// 来源: req.UserId 或 网关注入的 X-User-Id
	// 用于: Step 3.2 注销对应 Session 与在线状态
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 3.2: 调用业务层清理用户登录态
	// 来源: Step 3.1 确定的 userId
	// 用于: 返回登出成功标识
	err := s.userSvc.Logout(ctx, userId)
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

func buildRegisterResponse(rsp *userrsp.RegisterRespond) *authpb.RegisterResponse {
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
