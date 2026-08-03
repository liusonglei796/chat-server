package relation

import (
	"context"

	relationpb "kama_chat_server/api/gen/relation"
	applyreq "kama_chat_server/internal/dto/request/apply"
	groupreq "kama_chat_server/internal/dto/request/group"
	"kama_chat_server/internal/service/apply"
	"kama_chat_server/internal/service/friendship"
	"kama_chat_server/internal/service/group"
)

type GrpcServer struct {
	relationpb.UnimplementedRelationServiceServer
	friendshipSvc *friendship.FriendshipService
	groupSvc      *group.GroupService
	applySvc      *apply.ApplyService
}

func NewGrpcServer(friendshipSvc *friendship.FriendshipService, groupSvc *group.GroupService, applySvc *apply.ApplyService) *GrpcServer {
	return &GrpcServer{
		friendshipSvc: friendshipSvc,
		groupSvc:      groupSvc,
		applySvc:      applySvc,
	}
}

// Friendship
func (s *GrpcServer) GetFriendList(ctx context.Context, req *relationpb.GetFriendListRequest) (*relationpb.GetFriendListResponse, error) {
	list, total, err := s.friendshipSvc.GetFriendList(ctx, req.UserId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*relationpb.MyUserListRespond
	for _, v := range list {
		rspList = append(rspList, &relationpb.MyUserListRespond{
			UserId:   v.UserId,
			UserName: v.UserName,
			Avatar:   v.Avatar,
		})
	}
	return &relationpb.GetFriendListResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) GetFriendInfo(ctx context.Context, req *relationpb.GetFriendInfoRequest) (*relationpb.GetFriendInfoResponse, error) {
	info, err := s.friendshipSvc.GetFriendInfo(ctx, req.UserId, req.FriendId)
	if err != nil {
		return nil, err
	}
	return &relationpb.GetFriendInfoResponse{
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

func (s *GrpcServer) DeleteFriend(ctx context.Context, req *relationpb.DeleteFriendRequest) (*relationpb.DeleteFriendResponse, error) {
	err := s.friendshipSvc.DeleteFriend(ctx, req.UserId, req.FriendId)
	return &relationpb.DeleteFriendResponse{}, err
}

func (s *GrpcServer) BlackFriend(ctx context.Context, req *relationpb.BlackFriendRequest) (*relationpb.BlackFriendResponse, error) {
	err := s.friendshipSvc.BlackFriend(ctx, req.UserId, req.FriendId)
	return &relationpb.BlackFriendResponse{}, err
}

func (s *GrpcServer) UnblackFriend(ctx context.Context, req *relationpb.UnblackFriendRequest) (*relationpb.UnblackFriendResponse, error) {
	err := s.friendshipSvc.UnblackFriend(ctx, req.UserId, req.FriendId)
	return &relationpb.UnblackFriendResponse{}, err
}

func (s *GrpcServer) UpdateRemark(ctx context.Context, req *relationpb.UpdateRemarkRequest) (*relationpb.UpdateRemarkResponse, error) {
	err := s.friendshipSvc.UpdateRemark(ctx, req.UserId, req.FriendId, req.Remark)
	return &relationpb.UpdateRemarkResponse{}, err
}

// Group
func (s *GrpcServer) CreateGroup(ctx context.Context, req *relationpb.CreateGroupRequest) (*relationpb.CreateGroupResponse, error) {
	err := s.groupSvc.CreateGroup(ctx, req.OwnerId, groupreq.CreateGroupRequest{
		Name:    req.Name,
		Notice:  req.Notice,
		AddMode: int8(req.AddMode),
		Avatar:  req.Avatar,
	})
	return &relationpb.CreateGroupResponse{}, err
}

func (s *GrpcServer) LoadMyGroup(ctx context.Context, req *relationpb.LoadMyGroupRequest) (*relationpb.LoadMyGroupResponse, error) {
	list, total, err := s.groupSvc.LoadMyGroup(ctx, req.UserId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*relationpb.MyGroupListRespond
	for _, v := range list {
		rspList = append(rspList, &relationpb.MyGroupListRespond{
			GroupId:   v.GroupId,
			GroupName: v.GroupName,
			Avatar:    v.Avatar,
		})
	}
	return &relationpb.LoadMyGroupResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) GetGroupListByMember(ctx context.Context, req *relationpb.GetGroupListByMemberRequest) (*relationpb.GetGroupListByMemberResponse, error) {
	list, total, err := s.groupSvc.GetGroupListByMember(ctx, req.UserId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*relationpb.MyGroupListRespond
	for _, v := range list {
		rspList = append(rspList, &relationpb.MyGroupListRespond{
			GroupId:   v.GroupId,
			GroupName: v.GroupName,
			Avatar:    v.Avatar,
		})
	}
	return &relationpb.GetGroupListByMemberResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) CheckGroupAddMode(ctx context.Context, req *relationpb.CheckGroupAddModeRequest) (*relationpb.CheckGroupAddModeResponse, error) {
	mode, err := s.groupSvc.CheckGroupAddMode(ctx, req.GroupId)
	return &relationpb.CheckGroupAddModeResponse{AddMode: int32(mode)}, err
}

func (s *GrpcServer) LeaveGroup(ctx context.Context, req *relationpb.LeaveGroupRequest) (*relationpb.LeaveGroupResponse, error) {
	err := s.groupSvc.LeaveGroup(ctx, req.UserId, req.GroupId)
	return &relationpb.LeaveGroupResponse{}, err
}

func (s *GrpcServer) DismissGroup(ctx context.Context, req *relationpb.DismissGroupRequest) (*relationpb.DismissGroupResponse, error) {
	err := s.groupSvc.DismissGroup(ctx, req.OperatorId, req.GroupId)
	return &relationpb.DismissGroupResponse{}, err
}

func (s *GrpcServer) UpdateGroupInfo(ctx context.Context, req *relationpb.UpdateGroupInfoRequest) (*relationpb.UpdateGroupInfoResponse, error) {
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
	return &relationpb.UpdateGroupInfoResponse{}, err
}

func (s *GrpcServer) GetGroupMemberList(ctx context.Context, req *relationpb.GetGroupMemberListRequest) (*relationpb.GetGroupMemberListResponse, error) {
	list, total, err := s.groupSvc.GetGroupMemberList(ctx, req.UserId, req.GroupId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*relationpb.GetGroupMemberListRespond
	for _, v := range list {
		rspList = append(rspList, &relationpb.GetGroupMemberListRespond{
			UserId:   v.UserId,
			Nickname: v.Nickname,
			Avatar:   v.Avatar,
		})
	}
	return &relationpb.GetGroupMemberListResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) RemoveGroupMembers(ctx context.Context, req *relationpb.RemoveGroupMembersRequest) (*relationpb.RemoveGroupMembersResponse, error) {
	err := s.groupSvc.RemoveGroupMembers(ctx, req.OperatorId, groupreq.RemoveGroupMembersRequest{
		GroupId:  req.GroupId,
		UuidList: req.UuidList,
	})
	return &relationpb.RemoveGroupMembersResponse{}, err
}

func (s *GrpcServer) GetGroupDetail(ctx context.Context, req *relationpb.GetGroupDetailRequest) (*relationpb.GetGroupDetailResponse, error) {
	info, err := s.groupSvc.GetGroupDetail(ctx, req.UserId, req.GroupId)
	if err != nil {
		return nil, err
	}
	return &relationpb.GetGroupDetailResponse{
		GroupId:     info.GroupId,
		GroupName:   info.GroupName,
		GroupAvatar: info.GroupAvatar,
		GroupNotice: info.GroupNotice,
		MemberCnt:   int32(info.MemberCnt),
		OwnerId:     info.OwnerId,
		AddMode:     int32(info.AddMode),
	}, nil
}

func (s *GrpcServer) MuteMember(ctx context.Context, req *relationpb.MuteMemberRequest) (*relationpb.MuteMemberResponse, error) {
	err := s.groupSvc.MuteMember(ctx, req.OperatorId, groupreq.MuteMemberRequest{
		GroupId:  req.GroupId,
		UserId:   req.UserId,
		Duration: int(req.Duration),
	})
	return &relationpb.MuteMemberResponse{}, err
}

// CheckFriendship 返回好友关系状态：0=非好友 1=正常 2=已拉黑对方 3=被对方拉黑
func (s *GrpcServer) CheckFriendship(ctx context.Context, req *relationpb.CheckFriendshipRequest) (*relationpb.CheckFriendshipResponse, error) {
	fs, err := s.friendshipSvc.GetFriendshipStatus(ctx, req.UserId, req.FriendId)
	if err != nil {
		return nil, err
	}
	return &relationpb.CheckFriendshipResponse{Status: int32(fs)}, nil
}

// CheckGroupMember 检查用户是否为群成员
func (s *GrpcServer) CheckGroupMember(ctx context.Context, req *relationpb.CheckGroupMemberRequest) (*relationpb.CheckGroupMemberResponse, error) {
	isMember, err := s.groupSvc.IsGroupMember(ctx, req.GroupId, req.UserId)
	if err != nil {
		return nil, err
	}
	return &relationpb.CheckGroupMemberResponse{IsMember: isMember}, nil
}

// ListGroupMemberIds 获取群全部成员ID（不校验调用方身份，供消息服务群发）
func (s *GrpcServer) ListGroupMemberIds(ctx context.Context, req *relationpb.ListGroupMemberIdsRequest) (*relationpb.ListGroupMemberIdsResponse, error) {
	ids, err := s.groupSvc.ListGroupMemberIds(ctx, req.GroupId)
	if err != nil {
		return nil, err
	}
	return &relationpb.ListGroupMemberIdsResponse{UserIds: ids}, nil
}

// Apply
func (s *GrpcServer) ApplyFriend(ctx context.Context, req *relationpb.ApplyFriendRequest) (*relationpb.ApplyFriendResponse, error) {
	err := s.applySvc.ApplyFriend(ctx, req.UserId, applyreq.ApplyFriendRequest{
		FriendId: req.FriendId,
		Message:  req.Message,
	})
	return &relationpb.ApplyFriendResponse{}, err
}

func (s *GrpcServer) ApplyGroup(ctx context.Context, req *relationpb.ApplyGroupRequest) (*relationpb.ApplyGroupResponse, error) {
	err := s.applySvc.ApplyGroup(ctx, req.UserId, applyreq.ApplyGroupRequest{
		GroupId: req.GroupId,
		Message: req.Message,
	})
	return &relationpb.ApplyGroupResponse{}, err
}

func (s *GrpcServer) GetFriendApplyList(ctx context.Context, req *relationpb.GetFriendApplyListRequest) (*relationpb.GetFriendApplyListResponse, error) {
	rsp, err := s.applySvc.GetFriendApplyList(ctx, req.UserId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var list []*relationpb.FriendApplyListRespond
	for _, v := range rsp.List {
		list = append(list, &relationpb.FriendApplyListRespond{
			ApplicantId:     v.ApplicantId,
			ApplicantName:   v.ApplicantName,
			ApplicantAvatar: v.ApplicantAvatar,
			Message:         v.Message,
		})
	}
	return &relationpb.GetFriendApplyListResponse{Total: rsp.Total, List: list}, nil
}

func (s *GrpcServer) GetGroupApplyList(ctx context.Context, req *relationpb.GetGroupApplyListRequest) (*relationpb.GetGroupApplyListResponse, error) {
	rsp, err := s.applySvc.GetGroupApplyList(ctx, req.OperatorId, req.GroupId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var list []*relationpb.GroupApplyListRespond
	for _, v := range rsp.List {
		list = append(list, &relationpb.GroupApplyListRespond{
			ApplicantId:     v.ApplicantId,
			ApplicantName:   v.ApplicantName,
			ApplicantAvatar: v.ApplicantAvatar,
			Message:         v.Message,
		})
	}
	return &relationpb.GetGroupApplyListResponse{Total: rsp.Total, List: list}, nil
}

func (s *GrpcServer) PassFriendApply(ctx context.Context, req *relationpb.PassFriendApplyRequest) (*relationpb.PassFriendApplyResponse, error) {
	err := s.applySvc.PassFriendApply(ctx, req.UserId, req.ApplicantId)
	return &relationpb.PassFriendApplyResponse{}, err
}

func (s *GrpcServer) PassGroupApply(ctx context.Context, req *relationpb.PassGroupApplyRequest) (*relationpb.PassGroupApplyResponse, error) {
	err := s.applySvc.PassGroupApply(ctx, req.OperatorId, req.GroupId, req.ApplicantId)
	return &relationpb.PassGroupApplyResponse{}, err
}

func (s *GrpcServer) RefuseFriendApply(ctx context.Context, req *relationpb.RefuseFriendApplyRequest) (*relationpb.RefuseFriendApplyResponse, error) {
	err := s.applySvc.RefuseFriendApply(ctx, req.UserId, req.ApplicantId)
	return &relationpb.RefuseFriendApplyResponse{}, err
}

func (s *GrpcServer) RefuseGroupApply(ctx context.Context, req *relationpb.RefuseGroupApplyRequest) (*relationpb.RefuseGroupApplyResponse, error) {
	err := s.applySvc.RefuseGroupApply(ctx, req.OperatorId, req.GroupId, req.ApplicantId)
	return &relationpb.RefuseGroupApplyResponse{}, err
}

func (s *GrpcServer) BlackFriendApply(ctx context.Context, req *relationpb.BlackFriendApplyRequest) (*relationpb.BlackFriendApplyResponse, error) {
	err := s.applySvc.BlackFriendApply(ctx, req.UserId, req.ApplicantId)
	return &relationpb.BlackFriendApplyResponse{}, err
}

func (s *GrpcServer) BlackGroupApply(ctx context.Context, req *relationpb.BlackGroupApplyRequest) (*relationpb.BlackGroupApplyResponse, error) {
	err := s.applySvc.BlackGroupApply(ctx, req.OperatorId, req.GroupId, req.ApplicantId)
	return &relationpb.BlackGroupApplyResponse{}, err
}
