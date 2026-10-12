package group

import (
	"context"

	"github.com/go-kratos/kratos/v2/transport"

	grouppb "kama_chat_server/api/gen/group"
	groupreq "kama_chat_server/internal/common/dto/request/group"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
)

type GrpcServer struct {
	grouppb.UnimplementedGroupServiceServer
	groupSvc *GroupService
}

func NewGrpcServer(groupSvc *GroupService) *GrpcServer {
	return &GrpcServer{groupSvc: groupSvc}
}

// extractTrustedUserID 从 Kratos 传输上下文或 Context 中提取网关注入的受信用户身份
// 来源: Kratos HTTP transport 请求头 X-User-Id 或 Context 键值
// 用于: 群组操作权限校验与当前用户绑定
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

func (s *GrpcServer) CreateGroup(ctx context.Context, req *grouppb.CreateGroupRequest) (*grouppb.CreateGroupResponse, error) {
	// Step 1.1: 确认建群所有者身份
	// 来源: req.OwnerId 或 网关 X-User-Id
	// 用于: Step 1.2 执行创建群聊
	ownerId := req.OwnerId
	if ownerId == "" {
		ownerId = extractTrustedUserID(ctx)
	}
	if ownerId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// 提取客户端防重凭证：优先使用 req.ClientToken，若未传则尝试从 RPC metadata 中读取 X-Idempotency-Key
	clientToken := req.ClientToken
	if clientToken == "" {
		clientToken = interceptor.GetIdempotencyKeyFromContext(ctx)
	}

	// Step 1.2: 调用业务层执行创建群组
	// 来源: Step 1.1 的 ownerId 及群名称、公告、加群模式和 clientToken
	// 用于: 返回建群结果包含 group_id
	groupID, err := s.groupSvc.CreateGroup(ctx, ownerId, groupreq.CreateGroupRequest{
		Name:        req.Name,
		Notice:      req.Notice,
		AddMode:     int8(req.AddMode),
		Avatar:      req.Avatar,
		ClientToken: clientToken,
	})
	if err != nil {
		return nil, err
	}
	return &grouppb.CreateGroupResponse{GroupId: groupID}, nil
}

func (s *GrpcServer) LoadMyGroup(ctx context.Context, req *grouppb.LoadMyGroupRequest) (*grouppb.LoadMyGroupResponse, error) {
	// Step 2.1: 确认当前群主身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 2.2 分页查询我创建的群组
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

	// Step 2.2: 调用领域服务查询创建的群组列表
	// 来源: Step 2.1 的 userId 与分页条件
	// 用于: Step 2.3 组装列表响应
	list, total, err := s.groupSvc.LoadMyGroup(ctx, userId, page, pageSize)
	if err != nil {
		return nil, err
	}

	// Step 2.3: 封装输出群组列表协议
	// 来源: Step 2.2 返回的群组列表与总数
	// 用于: 返回给网关
	var rspList []*grouppb.MyGroupListRespond
	for _, v := range list {
		rspList = append(rspList, &grouppb.MyGroupListRespond{
			GroupId:   v.GroupId,
			GroupName: v.GroupName,
			Avatar:    v.Avatar,
		})
	}
	return &grouppb.LoadMyGroupResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) GetGroupListByMember(ctx context.Context, req *grouppb.GetGroupListByMemberRequest) (*grouppb.GetGroupListByMemberResponse, error) {
	// Step 3.1: 确认查询方用户身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 3.2 分页查询已加入的群聊列表
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

	// Step 3.2: 调用领域服务查询用户所在群列表
	// 来源: Step 3.1 的 userId 与分页条件
	// 用于: Step 3.3 组装列表响应
	list, total, err := s.groupSvc.GetGroupListByMember(ctx, userId, page, pageSize)
	if err != nil {
		return nil, err
	}

	// Step 3.3: 封装输出加入群组列表
	// 来源: Step 3.2 返回的群列表与条数
	// 用于: 返回给网关
	var rspList []*grouppb.MyGroupListRespond
	for _, v := range list {
		rspList = append(rspList, &grouppb.MyGroupListRespond{
			GroupId:   v.GroupId,
			GroupName: v.GroupName,
			Avatar:    v.Avatar,
		})
	}
	return &grouppb.GetGroupListByMemberResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) CheckGroupAddMode(ctx context.Context, req *grouppb.CheckGroupAddModeRequest) (*grouppb.CheckGroupAddModeResponse, error) {
	// Step 4.1: 查询目标群的加群验证方式
	// 来源: req.GroupId
	// 用于: 返回加群限制模式 (0:直接加入, 1:需验证等)
	mode, err := s.groupSvc.CheckGroupAddMode(ctx, req.GroupId)
	return &grouppb.CheckGroupAddModeResponse{AddMode: int32(mode)}, err
}

func (s *GrpcServer) LeaveGroup(ctx context.Context, req *grouppb.LeaveGroupRequest) (*grouppb.LeaveGroupResponse, error) {
	// Step 5.1: 确认退群用户身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 5.2 执行退出群聊
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 5.2: 调用业务层退出指定群组
	// 来源: Step 5.1 的 userId 与 req.GroupId
	// 用于: 返回退群结果
	err := s.groupSvc.LeaveGroup(ctx, userId, req.GroupId)
	return &grouppb.LeaveGroupResponse{}, err
}

func (s *GrpcServer) DismissGroup(ctx context.Context, req *grouppb.DismissGroupRequest) (*grouppb.DismissGroupResponse, error) {
	// Step 6.1: 确认解散操作人身份 (群主)
	// 来源: req.OperatorId 或 网关 X-User-Id
	// 用于: Step 6.2 权限校验并解散群聊
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 6.2: 调用业务层解散群组
	// 来源: Step 6.1 的 operatorId 与 req.GroupId
	// 用于: 返回解散结果
	err := s.groupSvc.DismissGroup(ctx, operatorId, req.GroupId)
	return &grouppb.DismissGroupResponse{}, err
}

func (s *GrpcServer) UpdateGroupInfo(ctx context.Context, req *grouppb.UpdateGroupInfoRequest) (*grouppb.UpdateGroupInfoResponse, error) {
	// Step 7.1: 确认修改资料的操作管理员身份
	// 来源: req.OperatorId 或 网关 X-User-Id
	// 用于: Step 7.2 组装更新参数并执行保存
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	updateReq := groupreq.UpdateGroupInfoRequest{
		Uuid: req.Uuid,
	}
	if req.Name != nil {
		updateReq.Name = req.Name
	}
	if req.Notice != nil {
		updateReq.Notice = req.Notice
	}
	if req.AddMode != nil {
		v := int8(*req.AddMode)
		updateReq.AddMode = &v
	}
	if req.Avatar != nil {
		updateReq.Avatar = req.Avatar
	}

	// Step 7.2: 调用领域服务更新群资料
	// 来源: Step 7.1 的 operatorId 与 updateReq
	// 用于: 返回修改结果
	err := s.groupSvc.UpdateGroupInfo(ctx, operatorId, updateReq)
	return &grouppb.UpdateGroupInfoResponse{}, err
}

func (s *GrpcServer) GetGroupMemberList(ctx context.Context, req *grouppb.GetGroupMemberListRequest) (*grouppb.GetGroupMemberListResponse, error) {
	// Step 8.1: 确认查看成员列表的操作人身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 8.2 分页查询群成员
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

	// Step 8.2: 调用领域服务获取群成员分页
	// 来源: Step 8.1 的 userId、req.GroupId 及分页参数
	// 用于: Step 8.3 组装成员响应列表
	list, total, err := s.groupSvc.GetGroupMemberList(ctx, userId, req.GroupId, page, pageSize)
	if err != nil {
		return nil, err
	}

	// Step 8.3: 封装输出群成员协议
	// 来源: Step 8.2 返回的数据与总数
	// 用于: 返回给网关
	var rspList []*grouppb.GetGroupMemberListRespond
	for _, v := range list {
		rspList = append(rspList, &grouppb.GetGroupMemberListRespond{
			UserId:   v.UserId,
			Nickname: v.Nickname,
			Avatar:   v.Avatar,
		})
	}
	return &grouppb.GetGroupMemberListResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) RemoveGroupMembers(ctx context.Context, req *grouppb.RemoveGroupMembersRequest) (*grouppb.RemoveGroupMembersResponse, error) {
	// Step 9.1: 确认踢人操作的管理员身份
	// 来源: req.OperatorId 或 网关 X-User-Id
	// 用于: Step 9.2 移除群成员
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 9.2: 调用业务层执行踢出成员
	// 来源: Step 9.1 的 operatorId、req.GroupId 与 req.UuidList
	// 用于: 返回移除结果
	err := s.groupSvc.RemoveGroupMembers(ctx, operatorId, groupreq.RemoveGroupMembersRequest{
		GroupId:  req.GroupId,
		UuidList: req.UuidList,
	})
	return &grouppb.RemoveGroupMembersResponse{}, err
}

func (s *GrpcServer) GetGroupDetail(ctx context.Context, req *grouppb.GetGroupDetailRequest) (*grouppb.GetGroupDetailResponse, error) {
	// Step 10.1: 确认查询人身份
	// 来源: req.UserId 或 网关 X-User-Id
	// 用于: Step 10.2 查询群聊详细信息
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 10.2: 调用领域服务查询群详情
	// 来源: Step 10.1 的 userId 与 req.GroupId
	// 用于: Step 10.3 组装群详情响应
	info, err := s.groupSvc.GetGroupDetail(ctx, userId, req.GroupId)
	if err != nil {
		return nil, err
	}

	// Step 10.3: 封装输出群详情协议数据
	// 来源: Step 10.2 返回的 info 实体
	// 用于: 返回给网关
	return &grouppb.GetGroupDetailResponse{
		GroupId:     info.GroupId,
		GroupName:   info.GroupName,
		GroupAvatar: info.GroupAvatar,
		GroupNotice: info.GroupNotice,
		MemberCnt:   int32(info.MemberCnt),
		OwnerId:     info.OwnerId,
		AddMode:     int32(info.AddMode),
	}, nil
}

func (s *GrpcServer) MuteMember(ctx context.Context, req *grouppb.MuteMemberRequest) (*grouppb.MuteMemberResponse, error) {
	// Step 11.1: 确认禁言操作管理员身份
	// 来源: req.OperatorId 或 网关 X-User-Id
	// 用于: Step 11.2 禁言群成员
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 11.2: 调用业务层执行禁言设置
	// 来源: Step 11.1 的 operatorId、目标用户与时长
	// 用于: 返回禁言结果
	err := s.groupSvc.MuteMember(ctx, operatorId, groupreq.MuteMemberRequest{
		GroupId:  req.GroupId,
		UserId:   req.UserId,
		Duration: int(req.Duration),
	})
	return &grouppb.MuteMemberResponse{}, err
}

func (s *GrpcServer) CheckGroupMember(ctx context.Context, req *grouppb.CheckGroupMemberRequest) (*grouppb.CheckGroupMemberResponse, error) {
	// Step 12.1: 检查指定用户是否为该群成员
	// 来源: req.GroupId 与 req.UserId
	// 用于: 跨服务校验发消息或访问权限
	isMember, err := s.groupSvc.IsGroupMember(ctx, req.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}
	return &grouppb.CheckGroupMemberResponse{IsMember: isMember}, nil
}

func (s *GrpcServer) ListGroupMemberIds(ctx context.Context, req *grouppb.ListGroupMemberIdsRequest) (*grouppb.ListGroupMemberIdsResponse, error) {
	// Step 13.1: 批量获取群全部成员 ID 列表
	// 来源: req.GroupId
	// 用于: 消息服务群发扩散下推
	ids, err := s.groupSvc.ListGroupMemberIds(ctx, req.GroupId)
	if err != nil {
		return nil, err
	}
	return &grouppb.ListGroupMemberIdsResponse{UserIds: ids}, nil
}

func (s *GrpcServer) GetGroupMemberRole(ctx context.Context, req *grouppb.GetGroupMemberRoleRequest) (*grouppb.GetGroupMemberRoleResponse, error) {
	// Step 14.1: 查询用户在群内的角色身份
	// 来源: req.GroupId 与 req.UserId
	// 用于: 权限控制 (0: 普通成员, 1: 管理员, 2: 群主)
	role, err := s.groupSvc.GetGroupMemberRole(ctx, req.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}
	return &grouppb.GetGroupMemberRoleResponse{Role: int32(role)}, nil
}

func (s *GrpcServer) AddGroupMember(ctx context.Context, req *grouppb.AddGroupMemberRequest) (*grouppb.AddGroupMemberResponse, error) {
	// Step 15.1: 跨服务直接新增群成员
	// 来源: req.GroupId、req.UserId 与 req.Role (由加群申请通过触发)
	// 用于: 持久化成员记录
	err := s.groupSvc.AddGroupMember(ctx, req.GroupId, req.UserId, int8(req.Role))
	return &grouppb.AddGroupMemberResponse{}, err
}

