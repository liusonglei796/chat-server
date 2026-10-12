package apply

import (
	"context"

	"github.com/go-kratos/kratos/v2/transport"

	applypb "kama_chat_server/api/gen/apply"
	applyreq "kama_chat_server/internal/common/dto/request/apply"
	"kama_chat_server/pkg/errorx"
)

type GrpcServer struct {
	applypb.UnimplementedApplyServiceServer
	applySvc *ApplyService
}

func NewGrpcServer(applySvc *ApplyService) *GrpcServer {
	return &GrpcServer{applySvc: applySvc}
}

// extractTrustedUserID 从 Kratos 传输上下文或 Context 中提取网关注入的受信用户身份
// 来源: Kratos HTTP transport 请求头 X-User-Id 或 Context 键值
// 用于: 申请操作发起人或群聊审核管理员身份绑定
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

func (s *GrpcServer) ApplyFriend(ctx context.Context, req *applypb.ApplyFriendRequest) (*applypb.ApplyFriendResponse, error) {
	// Step 1.1: 确认申请发起人身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 1.2 组装好友申请请求
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 1.2: 调用业务层执行好友申请
	// 来源: Step 1.1 的 userId 及申请目标与留言
	// 用于: 返回申请发起结果
	err := s.applySvc.ApplyFriend(ctx, userId, applyreq.ApplyFriendRequest{
		FriendId:    req.FriendId,
		Message:     req.Message,
		ClientToken: req.ClientToken,
	})
	return &applypb.ApplyFriendResponse{}, err
}

func (s *GrpcServer) ApplyGroup(ctx context.Context, req *applypb.ApplyGroupRequest) (*applypb.ApplyGroupResponse, error) {
	// Step 2.1: 确认申请加入群聊的用户身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 2.2 组装入群申请请求
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 2.2: 调用业务层执行加群申请
	// 来源: Step 2.1 的 userId 及目标群 ID 与验证信息
	// 用于: 返回入群申请结果
	err := s.applySvc.ApplyGroup(ctx, userId, applyreq.ApplyGroupRequest{
		GroupId:     req.GroupId,
		Message:     req.Message,
		ClientToken: req.ClientToken,
	})
	return &applypb.ApplyGroupResponse{}, err
}

func (s *GrpcServer) GetFriendApplyList(ctx context.Context, req *applypb.GetFriendApplyListRequest) (*applypb.GetFriendApplyListResponse, error) {
	// Step 3.1: 确认查询方用户身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 3.2 分页查询收到的好友申请
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

	// Step 3.2: 调用业务层查询好友申请列表
	// 来源: Step 3.1 的 userId 与分页条件
	// 用于: Step 3.3 组装申请列表响应
	rsp, err := s.applySvc.GetFriendApplyList(ctx, userId, page, pageSize)
	if err != nil {
		return nil, err
	}

	// Step 3.3: 封装输出申请列表协议
	// 来源: Step 3.2 返回的列表实体
	// 用于: 返回给网关或调用端
	var list []*applypb.FriendApplyListRespond
	for _, v := range rsp.List {
		list = append(list, &applypb.FriendApplyListRespond{
			ApplicantId:     v.ApplicantId,
			ApplicantName:   v.ApplicantName,
			ApplicantAvatar: v.ApplicantAvatar,
			Message:         v.Message,
		})
	}
	return &applypb.GetFriendApplyListResponse{Total: rsp.Total, List: list}, nil
}

func (s *GrpcServer) GetGroupApplyList(ctx context.Context, req *applypb.GetGroupApplyListRequest) (*applypb.GetGroupApplyListResponse, error) {
	// Step 4.1: 确认审核管理员身份
	// 来源: req.OperatorId 或 网关 X-User-Id
	// 用于: Step 4.2 查询群聊待审批申请
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
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

	// Step 4.2: 调用业务层查询入群申请列表
	// 来源: Step 4.1 的 operatorId、目标群组与分页条件
	// 用于: Step 4.3 组装列表响应
	rsp, err := s.applySvc.GetGroupApplyList(ctx, operatorId, req.GroupId, page, pageSize)
	if err != nil {
		return nil, err
	}

	// Step 4.3: 封装输出入群申请列表
	// 来源: Step 4.2 返回的数据实体
	// 用于: 返回给网关
	var list []*applypb.GroupApplyListRespond
	for _, v := range rsp.List {
		list = append(list, &applypb.GroupApplyListRespond{
			ApplicantId:     v.ApplicantId,
			ApplicantName:   v.ApplicantName,
			ApplicantAvatar: v.ApplicantAvatar,
			Message:         v.Message,
		})
	}
	return &applypb.GetGroupApplyListResponse{Total: rsp.Total, List: list}, nil
}

func (s *GrpcServer) PassFriendApply(ctx context.Context, req *applypb.PassFriendApplyRequest) (*applypb.PassFriendApplyResponse, error) {
	// Step 5.1: 确认审批人身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 5.2 执行好友申请通过与关系建立
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 5.2: 调用业务层同意好友申请
	// 来源: Step 5.1 确定的 userId 与 req.ApplicantId
	// 用于: 返回审批结果
	err := s.applySvc.PassFriendApply(ctx, userId, req.ApplicantId)
	return &applypb.PassFriendApplyResponse{}, err
}

func (s *GrpcServer) PassGroupApply(ctx context.Context, req *applypb.PassGroupApplyRequest) (*applypb.PassGroupApplyResponse, error) {
	// Step 6.1: 确认群管理员审批身份
	// 来源: req.OperatorId 或 网关 X-User-Id
	// 用于: Step 6.2 执行入群申请通过
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 6.2: 调用业务层同 Crypto 入群
	// 来源: Step 6.1 确定的 operatorId、req.GroupId 与 req.ApplicantId
	// 用于: 返回入群审批结果
	err := s.applySvc.PassGroupApply(ctx, operatorId, req.GroupId, req.ApplicantId)
	return &applypb.PassGroupApplyResponse{}, err
}

func (s *GrpcServer) RefuseFriendApply(ctx context.Context, req *applypb.RefuseFriendApplyRequest) (*applypb.RefuseFriendApplyResponse, error) {
	// Step 7.1: 确认拒绝操作人身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 7.2 执行拒绝好友申请
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 7.2: 调用业务层拒绝好友申请
	// 来源: Step 7.1 确定的 userId 与 req.ApplicantId
	// 用于: 返回拒绝结果
	err := s.applySvc.RefuseFriendApply(ctx, userId, req.ApplicantId)
	return &applypb.RefuseFriendApplyResponse{}, err
}

func (s *GrpcServer) RefuseGroupApply(ctx context.Context, req *applypb.RefuseGroupApplyRequest) (*applypb.RefuseGroupApplyResponse, error) {
	// Step 8.1: 确认群管理员拒绝操作身份
	// 来源: req.OperatorId 或 网关 X-User-Id
	// 用于: Step 8.2 执行拒绝入群申请
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 8.2: 调用业务层拒绝入群申请
	// 来源: Step 8.1 确定的 operatorId、req.GroupId 与 req.ApplicantId
	// 用于: 返回拒绝结果
	err := s.applySvc.RefuseGroupApply(ctx, operatorId, req.GroupId, req.ApplicantId)
	return &applypb.RefuseGroupApplyResponse{}, err
}

func (s *GrpcServer) BlackFriendApply(ctx context.Context, req *applypb.BlackFriendApplyRequest) (*applypb.BlackFriendApplyResponse, error) {
	// Step 9.1: 确认拉黑申请操作人身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 9.2 执行拉黑好友申请
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 9.2: 调用业务层拉黑好友申请
	// 来源: Step 9.1 确定的 userId 与 req.ApplicantId
	// 用于: 返回拉黑结果
	err := s.applySvc.BlackFriendApply(ctx, userId, req.ApplicantId)
	return &applypb.BlackFriendApplyResponse{}, err
}

func (s *GrpcServer) BlackGroupApply(ctx context.Context, req *applypb.BlackGroupApplyRequest) (*applypb.BlackGroupApplyResponse, error) {
	// Step 10.1: 确认群管理员拉黑操作身份
	// 来源: req.OperatorId 或 网关 X-User-Id
	// 用于: Step 10.2 执行拉黑入群申请
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 10.2: 调用业务层拉黑入群申请
	// 来源: Step 10.1 确定的 operatorId、req.GroupId 与 req.ApplicantId
	// 用于: 返回拉黑结果
	err := s.applySvc.BlackGroupApply(ctx, operatorId, req.GroupId, req.ApplicantId)
	return &applypb.BlackGroupApplyResponse{}, err
}
