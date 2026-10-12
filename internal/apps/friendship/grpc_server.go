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
// 来源: Kratos HTTP transport 请求头 X-User-Id 或 Context 键值
// 用于: 好友业务操作权限校验与当前用户绑定
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
	// Step 1.1: 确认请求操作方用户身份
	// 来源: req.UserId 或 网关注入的 X-User-Id
	// 用于: Step 1.2 查询用户好友列表
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

	// Step 1.2: 调用业务层获取好友分页数据
	// 来源: Step 1.1 的 userId 及分页参数
	// 用于: Step 1.3 组装好友列表响应
	list, total, err := s.friendshipSvc.GetFriendList(ctx, userId, page, pageSize)
	if err != nil {
		return nil, err
	}

	// Step 1.3: 封装输出列表数据
	// 来源: Step 1.2 查出的好友列表与总条数
	// 用于: 返回给网关或调用方
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
	// Step 2.1: 确认当前用户与目标好友身份
	// 来源: req.UserId / 网关 X-User-Id 及 req.FriendId
	// 用于: Step 2.2 查询对应好友详细信息
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 2.2: 调用领域服务获取好友详情
	// 来源: Step 2.1 确认的 userId 与 req.FriendId
	// 用于: Step 2.3 组装好友信息详情响应
	info, err := s.friendshipSvc.GetFriendInfo(ctx, userId, req.FriendId)
	if err != nil {
		return nil, err
	}

	// Step 2.3: 组装输出好友详情响应
	// 来源: Step 2.2 返回的 info 实体
	// 用于: 返回给上游网关
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
	// Step 3.1: 确认删除发起方身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 3.2 执行双向解绑逻辑
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 3.2: 调用领域服务删除好友关系
	// 来源: Step 3.1 确定的 userId 与 req.FriendId
	// 用于: 返回删除结果
	err := s.friendshipSvc.DeleteFriend(ctx, userId, req.FriendId)
	return &friendshippb.DeleteFriendResponse{}, err
}

func (s *GrpcServer) BlackFriend(ctx context.Context, req *friendshippb.BlackFriendRequest) (*friendshippb.BlackFriendResponse, error) {
	// Step 4.1: 确认拉黑操作方身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 4.2 执行拉黑逻辑
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 4.2: 调用领域服务拉黑指定好友
	// 来源: Step 4.1 确定的 userId 与 req.FriendId
	// 用于: 返回拉黑结果
	err := s.friendshipSvc.BlackFriend(ctx, userId, req.FriendId)
	return &friendshippb.BlackFriendResponse{}, err
}

func (s *GrpcServer) UnblackFriend(ctx context.Context, req *friendshippb.UnblackFriendRequest) (*friendshippb.UnblackFriendResponse, error) {
	// Step 5.1: 确认取消拉黑操作方身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 5.2 执行恢复好友关系逻辑
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 5.2: 调用领域服务解除拉黑状态
	// 来源: Step 5.1 确定的 userId 与 req.FriendId
	// 用于: 返回解除结果
	err := s.friendshipSvc.UnblackFriend(ctx, userId, req.FriendId)
	return &friendshippb.UnblackFriendResponse{}, err
}

func (s *GrpcServer) UpdateRemark(ctx context.Context, req *friendshippb.UpdateRemarkRequest) (*friendshippb.UpdateRemarkResponse, error) {
	// Step 6.1: 确认修改备注操作方身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 6.2 执行备注更新
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 6.2: 调用领域服务更新备注信息
	// 来源: Step 6.1 确定的 userId、req.FriendId 及 req.Remark
	// 用于: 返回修改结果
	err := s.friendshipSvc.UpdateRemark(ctx, userId, req.FriendId, req.Remark)
	return &friendshippb.UpdateRemarkResponse{}, err
}

func (s *GrpcServer) CheckFriendship(ctx context.Context, req *friendshippb.CheckFriendshipRequest) (*friendshippb.CheckFriendshipResponse, error) {
	// Step 7.1: 调用领域服务检查双方好友关系状态
	// 来源: req.UserId 与 req.FriendId
	// 用于: Step 7.2 返回状态枚举码
	fs, err := s.friendshipSvc.GetFriendshipStatus(ctx, req.UserId, req.FriendId)
	if err != nil {
		return nil, err
	}
	return &friendshippb.CheckFriendshipResponse{Status: int32(fs)}, nil
}

func (s *GrpcServer) EstablishFriendship(ctx context.Context, req *friendshippb.EstablishFriendshipRequest) (*friendshippb.EstablishFriendshipResponse, error) {
	// Step 8.1: 执行跨服务好友关系双向建立
	// 来源: req.UserId 与 req.FriendId (由 apply 审核通过触发)
	// 用于: 持久化并广播关系建立事件
	err := s.friendshipSvc.EstablishFriendship(ctx, req.UserId, req.FriendId)
	return &friendshippb.EstablishFriendshipResponse{}, err
}
