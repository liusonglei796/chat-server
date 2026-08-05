package group

import (
	"context"

	grouppb "kama_chat_server/api/gen/group"
	groupreq "kama_chat_server/internal/common/dto/request/group"
)

type GrpcServer struct {
	grouppb.UnimplementedGroupServiceServer
	groupSvc *GroupService
}

func NewGrpcServer(groupSvc *GroupService) *GrpcServer {
	return &GrpcServer{groupSvc: groupSvc}
}

func (s *GrpcServer) CreateGroup(ctx context.Context, req *grouppb.CreateGroupRequest) (*grouppb.CreateGroupResponse, error) {
	err := s.groupSvc.CreateGroup(ctx, req.OwnerId, groupreq.CreateGroupRequest{
		Name:    req.Name,
		Notice:  req.Notice,
		AddMode: int8(req.AddMode),
		Avatar:  req.Avatar,
	})
	return &grouppb.CreateGroupResponse{}, err
}

func (s *GrpcServer) LoadMyGroup(ctx context.Context, req *grouppb.LoadMyGroupRequest) (*grouppb.LoadMyGroupResponse, error) {
	list, total, err := s.groupSvc.LoadMyGroup(ctx, req.UserId, int(req.Page), int(req.PageSize))
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
	list, total, err := s.groupSvc.GetGroupListByMember(ctx, req.UserId, int(req.Page), int(req.PageSize))
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
	mode, err := s.groupSvc.CheckGroupAddMode(ctx, req.GroupId)
	return &grouppb.CheckGroupAddModeResponse{AddMode: int32(mode)}, err
}

func (s *GrpcServer) LeaveGroup(ctx context.Context, req *grouppb.LeaveGroupRequest) (*grouppb.LeaveGroupResponse, error) {
	err := s.groupSvc.LeaveGroup(ctx, req.UserId, req.GroupId)
	return &grouppb.LeaveGroupResponse{}, err
}

func (s *GrpcServer) DismissGroup(ctx context.Context, req *grouppb.DismissGroupRequest) (*grouppb.DismissGroupResponse, error) {
	err := s.groupSvc.DismissGroup(ctx, req.OperatorId, req.GroupId)
	return &grouppb.DismissGroupResponse{}, err
}

func (s *GrpcServer) UpdateGroupInfo(ctx context.Context, req *grouppb.UpdateGroupInfoRequest) (*grouppb.UpdateGroupInfoResponse, error) {
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
	err := s.groupSvc.UpdateGroupInfo(ctx, req.OperatorId, updateReq)
	return &grouppb.UpdateGroupInfoResponse{}, err
}

func (s *GrpcServer) GetGroupMemberList(ctx context.Context, req *grouppb.GetGroupMemberListRequest) (*grouppb.GetGroupMemberListResponse, error) {
	list, total, err := s.groupSvc.GetGroupMemberList(ctx, req.UserId, req.GroupId, int(req.Page), int(req.PageSize))
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
	err := s.groupSvc.RemoveGroupMembers(ctx, req.OperatorId, groupreq.RemoveGroupMembersRequest{
		GroupId:  req.GroupId,
		UuidList: req.UuidList,
	})
	return &grouppb.RemoveGroupMembersResponse{}, err
}

func (s *GrpcServer) GetGroupDetail(ctx context.Context, req *grouppb.GetGroupDetailRequest) (*grouppb.GetGroupDetailResponse, error) {
	info, err := s.groupSvc.GetGroupDetail(ctx, req.UserId, req.GroupId)
	if err != nil {
		return nil, err
	}
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
	err := s.groupSvc.MuteMember(ctx, req.OperatorId, groupreq.MuteMemberRequest{
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
