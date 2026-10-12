package user

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	userpb "kama_chat_server/api/gen/user"
	"kama_chat_server/internal/common/dto/request/user"
	"kama_chat_server/pkg/errorx"
)

type GrpcServer struct {
	userpb.UnimplementedUserServiceServer
	svc *UserService
}

func NewGrpcServer(svc *UserService) *GrpcServer {
	return &GrpcServer{svc: svc}
}

// extractTrustedUserID 从 Kratos 传输上下文或 Context 中提取网关注入的受信用户身份
// 来源: Kratos HTTP transport 请求头 X-User-Id 或 Context 键值
// 用于: 用户信息更新与查询的身份验证
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

func (s *GrpcServer) UpdateUserInfo(ctx context.Context, req *userpb.UpdateUserInfoRequest) (*userpb.UpdateUserInfoResponse, error) {
	// Step 1.1: 解析与绑定操作用户身份
	// 来源: gRPC 请求字段 req.UserId 或 网关注入的 X-User-Id
	// 用于: Step 1.2 构建业务更新参数
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 1.2: 组装用户信息更新 DTO
	// 来源: Step 1.1 确认的 userId 及请求中传入的资料字段
	// 用于: Step 1.3 提交至 UserService 完成持久化
	updateReq := user.UpdateUserInfoRequest{
		Email:     req.Email,
		Nickname:  req.Nickname,
		Birthday:  req.Birthday,
		Signature: req.Signature,
		Avatar:    req.Avatar,
	}

	// Step 1.3: 调用业务领域层更新用户信息
	// 来源: Step 1.2 组装的 updateReq
	// 用于: 返回更新响应给客户端或网关
	err := s.svc.UpdateUserInfo(ctx, userId, updateReq)
	if err != nil {
		return nil, err
	}
	return &userpb.UpdateUserInfoResponse{}, nil
}

func (s *GrpcServer) GetUserInfo(ctx context.Context, req *userpb.GetUserInfoRequest) (*userpb.GetUserInfoResponse, error) {
	// Step 2.1: 提取请求方身份并推导目标用户
	// 来源: req.RequesterId、req.TargetId 或网关注入的 X-User-Id
	// 用于: Step 2.2 领域层用户详情查询
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

	// Step 2.2: 调用业务层获取用户详细资料
	// 来源: Step 2.1 推导出的 requesterId 与 targetId
	// 用于: Step 2.3 序列化并组装响应协议
	info, err := s.svc.GetUserInfo(ctx, requesterId, targetId)
	if err != nil {
		return nil, err
	}

	// Step 2.3: 封装 gRPC / HTTP 响应
	// 来源: Step 2.2 返回的 info 实体
	// 用于: 输出给上游网关或微服务客户端
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

func (s *GrpcServer) BatchGetPublicUserInfo(ctx context.Context, req *userpb.BatchGetPublicUserInfoRequest) (*userpb.BatchGetPublicUserInfoResponse, error) {
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

func (s *GrpcServer) GetUserStatus(ctx context.Context, req *userpb.GetUserStatusRequest) (*userpb.GetUserStatusResponse, error) {
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

func (s *GrpcServer) KickUser(ctx context.Context, req *userpb.KickUserRequest) (*userpb.KickUserResponse, error) {
	err := s.svc.KickUser(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return &userpb.KickUserResponse{}, nil
}
