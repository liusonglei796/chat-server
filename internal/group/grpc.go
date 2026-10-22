package group

import (
	"context"

	"github.com/go-kratos/kratos/v2/transport"

	grouppb "kama_chat_server/api/gen/group"
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
	ownerId := req.OwnerId
	if ownerId == "" {
		ownerId = extractTrustedUserID(ctx)
	}
	if ownerId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	clientToken := req.ClientToken
	if clientToken == "" {
		clientToken = interceptor.GetIdempotencyKeyFromContext(ctx)
	}

	groupID, err := s.groupSvc.CreateGroup(ctx, ownerId, CreateGroupRequest{
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

	list, total, err := s.groupSvc.LoadMyGroup(ctx, userId, page, pageSize)
	if err != nil {
		return nil, err
	}

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

	list, total, err := s.groupSvc.GetGroupListByMember(ctx, userId, page, pageSize)
	if err != nil {
		return nil, err
	}

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
	addMode, err := s.groupSvc.CheckGroupAddMode(ctx, req.GroupId)
	if err != nil {
		return nil, err
	}
	return &grouppb.CheckGroupAddModeResponse{AddMode: int32(addMode)}, nil
}

func (s *GrpcServer) LeaveGroup(ctx context.Context, req *grouppb.LeaveGroupRequest) (*grouppb.LeaveGroupResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.groupSvc.LeaveGroup(ctx, userId, req.GroupId)
	return &grouppb.LeaveGroupResponse{}, err
}

func (s *GrpcServer) DismissGroup(ctx context.Context, req *grouppb.DismissGroupRequest) (*grouppb.DismissGroupResponse, error) {
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.groupSvc.DismissGroup(ctx, operatorId, req.GroupId)
	return &grouppb.DismissGroupResponse{}, err
}

func (s *GrpcServer) UpdateGroupInfo(ctx context.Context, req *grouppb.UpdateGroupInfoRequest) (*grouppb.UpdateGroupInfoResponse, error) {
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	updateReq := UpdateGroupInfoRequest{
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

	err := s.groupSvc.UpdateGroupInfo(ctx, operatorId, updateReq)
	return &grouppb.UpdateGroupInfoResponse{}, err
}

func (s *GrpcServer) GetGroupMemberList(ctx context.Context, req *grouppb.GetGroupMemberListRequest) (*grouppb.GetGroupMemberListResponse, error) {
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

	list, total, err := s.groupSvc.GetGroupMemberList(ctx, userId, req.GroupId, page, pageSize)
	if err != nil {
		return nil, err
	}

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
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.groupSvc.RemoveGroupMembers(ctx, operatorId, RemoveGroupMembersRequest{
		GroupId:  req.GroupId,
		UuidList: req.UuidList,
	})
	return &grouppb.RemoveGroupMembersResponse{}, err
}

func (s *GrpcServer) GetGroupDetail(ctx context.Context, req *grouppb.GetGroupDetailRequest) (*grouppb.GetGroupDetailResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	detail, err := s.groupSvc.GetGroupDetail(ctx, userId, req.GroupId)
	if err != nil {
		return nil, err
	}

	return &grouppb.GetGroupDetailResponse{
		GroupId:     detail.GroupId,
		GroupName:   detail.GroupName,
		GroupAvatar: detail.GroupAvatar,
		GroupNotice: detail.GroupNotice,
		MemberCnt:   int32(detail.MemberCnt),
		OwnerId:     detail.OwnerId,
		AddMode:     int32(detail.AddMode),
	}, nil
}

func (s *GrpcServer) MuteMember(ctx context.Context, req *grouppb.MuteMemberRequest) (*grouppb.MuteMemberResponse, error) {
	operatorId := req.OperatorId
	if operatorId == "" {
		operatorId = extractTrustedUserID(ctx)
	}
	if operatorId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.groupSvc.MuteMember(ctx, operatorId, MuteMemberRequest{
		GroupId:  req.GroupId,
		UserId:   req.UserId,
		Duration: int(req.Duration),
	})
	return &grouppb.MuteMemberResponse{}, err
}

func (s *GrpcServer) CheckGroupMember(ctx context.Context, req *grouppb.CheckGroupMemberRequest) (*grouppb.CheckGroupMemberResponse, error) {
	isMember, err := s.groupSvc.IsGroupMember(ctx, req.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}
	return &grouppb.CheckGroupMemberResponse{IsMember: isMember}, nil
}

func (s *GrpcServer) ListGroupMemberIds(ctx context.Context, req *grouppb.ListGroupMemberIdsRequest) (*grouppb.ListGroupMemberIdsResponse, error) {
	ids, err := s.groupSvc.ListGroupMemberIds(ctx, req.GroupId)
	if err != nil {
		return nil, err
	}
	return &grouppb.ListGroupMemberIdsResponse{UserIds: ids}, nil
}

func (s *GrpcServer) GetGroupMemberRole(ctx context.Context, req *grouppb.GetGroupMemberRoleRequest) (*grouppb.GetGroupMemberRoleResponse, error) {
	role, err := s.groupSvc.GetGroupMemberRole(ctx, req.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}
	return &grouppb.GetGroupMemberRoleResponse{Role: int32(role)}, nil
}

func (s *GrpcServer) AddGroupMember(ctx context.Context, req *grouppb.AddGroupMemberRequest) (*grouppb.AddGroupMemberResponse, error) {
	err := s.groupSvc.AddGroupMember(ctx, req.GroupId, req.UserId, int8(req.Role))
	return &grouppb.AddGroupMemberResponse{}, err
}
