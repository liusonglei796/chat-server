package apply

import (
	"context"

	applypb "kama_chat_server/api/gen/apply"
	applyreq "kama_chat_server/internal/common/dto/request/apply"
)

type GrpcServer struct {
	applypb.UnimplementedApplyServiceServer
	applySvc *ApplyService
}

func NewGrpcServer(applySvc *ApplyService) *GrpcServer {
	return &GrpcServer{applySvc: applySvc}
}

func (s *GrpcServer) ApplyFriend(ctx context.Context, req *applypb.ApplyFriendRequest) (*applypb.ApplyFriendResponse, error) {
	err := s.applySvc.ApplyFriend(ctx, req.UserId, applyreq.ApplyFriendRequest{
		FriendId: req.FriendId,
		Message:  req.Message,
	})
	return &applypb.ApplyFriendResponse{}, err
}

func (s *GrpcServer) ApplyGroup(ctx context.Context, req *applypb.ApplyGroupRequest) (*applypb.ApplyGroupResponse, error) {
	err := s.applySvc.ApplyGroup(ctx, req.UserId, applyreq.ApplyGroupRequest{
		GroupId: req.GroupId,
		Message: req.Message,
	})
	return &applypb.ApplyGroupResponse{}, err
}

func (s *GrpcServer) GetFriendApplyList(ctx context.Context, req *applypb.GetFriendApplyListRequest) (*applypb.GetFriendApplyListResponse, error) {
	rsp, err := s.applySvc.GetFriendApplyList(ctx, req.UserId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
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
	rsp, err := s.applySvc.GetGroupApplyList(ctx, req.OperatorId, req.GroupId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
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
	err := s.applySvc.PassFriendApply(ctx, req.UserId, req.ApplicantId)
	return &applypb.PassFriendApplyResponse{}, err
}

func (s *GrpcServer) PassGroupApply(ctx context.Context, req *applypb.PassGroupApplyRequest) (*applypb.PassGroupApplyResponse, error) {
	err := s.applySvc.PassGroupApply(ctx, req.OperatorId, req.GroupId, req.ApplicantId)
	return &applypb.PassGroupApplyResponse{}, err
}

func (s *GrpcServer) RefuseFriendApply(ctx context.Context, req *applypb.RefuseFriendApplyRequest) (*applypb.RefuseFriendApplyResponse, error) {
	err := s.applySvc.RefuseFriendApply(ctx, req.UserId, req.ApplicantId)
	return &applypb.RefuseFriendApplyResponse{}, err
}

func (s *GrpcServer) RefuseGroupApply(ctx context.Context, req *applypb.RefuseGroupApplyRequest) (*applypb.RefuseGroupApplyResponse, error) {
	err := s.applySvc.RefuseGroupApply(ctx, req.OperatorId, req.GroupId, req.ApplicantId)
	return &applypb.RefuseGroupApplyResponse{}, err
}

func (s *GrpcServer) BlackFriendApply(ctx context.Context, req *applypb.BlackFriendApplyRequest) (*applypb.BlackFriendApplyResponse, error) {
	err := s.applySvc.BlackFriendApply(ctx, req.UserId, req.ApplicantId)
	return &applypb.BlackFriendApplyResponse{}, err
}

func (s *GrpcServer) BlackGroupApply(ctx context.Context, req *applypb.BlackGroupApplyRequest) (*applypb.BlackGroupApplyResponse, error) {
	err := s.applySvc.BlackGroupApply(ctx, req.OperatorId, req.GroupId, req.ApplicantId)
	return &applypb.BlackGroupApplyResponse{}, err
}
