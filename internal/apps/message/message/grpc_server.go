package message

import (
	"context"

	messagepb "kama_chat_server/api/gen/message"
	messagereq "kama_chat_server/internal/common/dto/request/message"
	sessionreq "kama_chat_server/internal/common/dto/request/session"
	"kama_chat_server/internal/apps/message/session"
	"strconv"
)

type GrpcServer struct {
	messagepb.UnimplementedMessageServiceServer
	msgSvc     *MessageService
	sessionSvc *session.SessionService
}

func NewGrpcServer(msgSvc *MessageService, sessionSvc *session.SessionService) *GrpcServer {
	return &GrpcServer{
		msgSvc:     msgSvc,
		sessionSvc: sessionSvc,
	}
}

// Message
func (s *GrpcServer) SendMessage(ctx context.Context, req *messagepb.SendMessageRequest) (*messagepb.SendMessageResponse, error) {
	chatReq := messagereq.ChatMessageRequest{
		SessionId:   req.SessionId,
		Type:        int8(req.Type),
		Content:     req.Content,
		Url:         req.Url,
		SendId:      req.SendId,
		SendName:    req.SendName,
		SendAvatar:  req.SendAvatar,
		ReceiveId:   req.ReceiveId,
		FileSize:    req.FileSize,
		FileType:    req.FileType,
		FileName:    req.FileName,
		AVdata:      req.AvData,
		ClientMsgId: req.ClientMsgId,
	}
	rsp, err := s.msgSvc.SendMessage(ctx, chatReq)
	if err != nil {
		return nil, err
	}
	return &messagepb.SendMessageResponse{
		MessageUuid: rsp.MessageUuid,
		CreatedAt:   rsp.CreatedAt,
	}, nil
}

func (s *GrpcServer) GetMessageList(ctx context.Context, req *messagepb.GetMessageListRequest) (*messagepb.GetMessageListResponse, error) {
	list, total, err := s.msgSvc.GetMessageList(ctx, req.RequesterId, req.PartnerId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*messagepb.GetMessageListRespond
	for _, v := range list {
		rspList = append(rspList, &messagepb.GetMessageListRespond{
			SendId:     v.SendId,
			SendName:   v.SendName,
			SendAvatar: v.SendAvatar,
			ReceiveId:  v.ReceiveId,
			Content:    v.Content,
			Url:        v.Url,
			Type:       int32(v.Type),
			FileType:   v.FileType,
			FileName:   v.FileName,
			FileSize:   func() int64 { i, _ := strconv.ParseInt(v.FileSize, 10, 64); return i }(),
			CreatedAt:  v.CreatedAt,
		})
	}
	return &messagepb.GetMessageListResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) GetGroupMessageList(ctx context.Context, req *messagepb.GetGroupMessageListRequest) (*messagepb.GetGroupMessageListResponse, error) {
	list, total, err := s.msgSvc.GetGroupMessageList(ctx, req.UserId, req.GroupId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*messagepb.GetMessageListRespond
	for _, v := range list {
		rspList = append(rspList, &messagepb.GetMessageListRespond{
			SendId:     v.SendId,
			SendName:   v.SendName,
			SendAvatar: v.SendAvatar,
			ReceiveId:  v.ReceiveId,
			Content:    v.Content,
			Url:        v.Url,
			Type:       int32(v.Type),
			FileType:   v.FileType,
			FileName:   v.FileName,
			FileSize:   func() int64 { i, _ := strconv.ParseInt(v.FileSize, 10, 64); return i }(),
			CreatedAt:  v.CreatedAt,
		})
	}
	return &messagepb.GetGroupMessageListResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) GetMessageListCursor(ctx context.Context, req *messagepb.GetMessageListCursorRequest) (*messagepb.GetMessageListCursorResponse, error) {
	list, next, more, err := s.msgSvc.GetMessageListCursor(ctx, req.RequesterId, req.PartnerId, req.Cursor, int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*messagepb.GetMessageListRespond
	for _, v := range list {
		rspList = append(rspList, &messagepb.GetMessageListRespond{
			SendId:     v.SendId,
			SendName:   v.SendName,
			SendAvatar: v.SendAvatar,
			ReceiveId:  v.ReceiveId,
			Content:    v.Content,
			Url:        v.Url,
			Type:       int32(v.Type),
			FileType:   v.FileType,
			FileName:   v.FileName,
			FileSize:   func() int64 { i, _ := strconv.ParseInt(v.FileSize, 10, 64); return i }(),
			CreatedAt:  v.CreatedAt,
		})
	}
	return &messagepb.GetMessageListCursorResponse{List: rspList, NextCursor: next, HasMore: more}, nil
}

func (s *GrpcServer) GetGroupMessageListCursor(ctx context.Context, req *messagepb.GetGroupMessageListCursorRequest) (*messagepb.GetGroupMessageListCursorResponse, error) {
	list, next, more, err := s.msgSvc.GetGroupMessageListCursor(ctx, req.UserId, req.GroupId, req.Cursor, int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*messagepb.GetMessageListRespond
	for _, v := range list {
		rspList = append(rspList, &messagepb.GetMessageListRespond{
			SendId:     v.SendId,
			SendName:   v.SendName,
			SendAvatar: v.SendAvatar,
			ReceiveId:  v.ReceiveId,
			Content:    v.Content,
			Url:        v.Url,
			Type:       int32(v.Type),
			FileType:   v.FileType,
			FileName:   v.FileName,
			FileSize:   func() int64 { i, _ := strconv.ParseInt(v.FileSize, 10, 64); return i }(),
			CreatedAt:  v.CreatedAt,
		})
	}
	return &messagepb.GetGroupMessageListCursorResponse{List: rspList, NextCursor: next, HasMore: more}, nil
}

func (s *GrpcServer) RecallMessage(ctx context.Context, req *messagepb.RecallMessageRequest) (*messagepb.RecallMessageResponse, error) {
	err := s.msgSvc.RecallMessage(ctx, req.UserId, messagereq.RecallMessageRequest{MessageUuid: req.MessageUuid})
	return &messagepb.RecallMessageResponse{}, err
}

// Session
func (s *GrpcServer) CreateSession(ctx context.Context, req *messagepb.CreateSessionRequest) (*messagepb.CreateSessionResponse, error) {
	sid, err := s.sessionSvc.CreateSession(ctx, req.SendId, req.ReceiveId)
	return &messagepb.CreateSessionResponse{SessionId: sid}, err
}

func (s *GrpcServer) CheckOpenSessionAllowed(ctx context.Context, req *messagepb.CheckOpenSessionAllowedRequest) (*messagepb.CheckOpenSessionAllowedResponse, error) {
	allowed, err := s.sessionSvc.CheckOpenSessionAllowed(ctx, req.SendId, req.ReceiveId)
	return &messagepb.CheckOpenSessionAllowedResponse{Allowed: allowed}, err
}

func (s *GrpcServer) OpenSession(ctx context.Context, req *messagepb.OpenSessionRequest) (*messagepb.OpenSessionResponse, error) {
	sid, err := s.sessionSvc.OpenSession(ctx, req.SendId, sessionreq.OpenSessionRequest{ReceiveId: req.ReceiveId})
	return &messagepb.OpenSessionResponse{SessionId: sid}, err
}

func (s *GrpcServer) GetUserSessionList(ctx context.Context, req *messagepb.GetUserSessionListRequest) (*messagepb.GetUserSessionListResponse, error) {
	list, total, err := s.sessionSvc.GetUserSessionList(ctx, req.OwnerId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*messagepb.UserSessionListRespond
	for _, v := range list {
		rspList = append(rspList, &messagepb.UserSessionListRespond{
			SessionId:       v.SessionId,
			Avatar:          v.Avatar,
			UserId:          v.UserId,
			Username:        v.Username,
			LastMessage:     v.LastMessage,
			LastMessageTime: v.LastMessageTime,
			LastMessageType: int32(v.LastMessageType),
			IsPinned:        func() int32 { if v.IsPinned { return 1 }; return 0 }(),
		})
	}
	return &messagepb.GetUserSessionListResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) GetGroupSessionList(ctx context.Context, req *messagepb.GetGroupSessionListRequest) (*messagepb.GetGroupSessionListResponse, error) {
	list, total, err := s.sessionSvc.GetGroupSessionList(ctx, req.OwnerId, int(req.Page), int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*messagepb.GroupSessionListRespond
	for _, v := range list {
		rspList = append(rspList, &messagepb.GroupSessionListRespond{
			SessionId:       v.SessionId,
			Avatar:          v.Avatar,
			GroupId:         v.GroupId,
			GroupName:       v.GroupName,
			LastMessage:     v.LastMessage,
			LastMessageTime: v.LastMessageTime,
			LastMessageType: int32(v.LastMessageType),
			IsPinned:        func() int32 { if v.IsPinned { return 1 }; return 0 }(),
		})
	}
	return &messagepb.GetGroupSessionListResponse{Total: total, List: rspList}, nil
}

func (s *GrpcServer) GetUserSessionListCursor(ctx context.Context, req *messagepb.GetUserSessionListCursorRequest) (*messagepb.GetUserSessionListCursorResponse, error) {
	list, next, more, err := s.sessionSvc.GetUserSessionListCursor(ctx, req.OwnerId, req.Cursor, int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*messagepb.UserSessionListRespond
	for _, v := range list {
		rspList = append(rspList, &messagepb.UserSessionListRespond{
			SessionId:       v.SessionId,
			Avatar:          v.Avatar,
			UserId:          v.UserId,
			Username:        v.Username,
			LastMessage:     v.LastMessage,
			LastMessageTime: v.LastMessageTime,
			LastMessageType: int32(v.LastMessageType),
			IsPinned:        func() int32 { if v.IsPinned { return 1 }; return 0 }(),
		})
	}
	return &messagepb.GetUserSessionListCursorResponse{List: rspList, NextCursor: next, HasMore: more}, nil
}

func (s *GrpcServer) GetGroupSessionListCursor(ctx context.Context, req *messagepb.GetGroupSessionListCursorRequest) (*messagepb.GetGroupSessionListCursorResponse, error) {
	list, next, more, err := s.sessionSvc.GetGroupSessionListCursor(ctx, req.OwnerId, req.Cursor, int(req.PageSize))
	if err != nil {
		return nil, err
	}
	var rspList []*messagepb.GroupSessionListRespond
	for _, v := range list {
		rspList = append(rspList, &messagepb.GroupSessionListRespond{
			SessionId:       v.SessionId,
			Avatar:          v.Avatar,
			GroupId:         v.GroupId,
			GroupName:       v.GroupName,
			LastMessage:     v.LastMessage,
			LastMessageTime: v.LastMessageTime,
			LastMessageType: int32(v.LastMessageType),
			IsPinned:        func() int32 { if v.IsPinned { return 1 }; return 0 }(),
		})
	}
	return &messagepb.GetGroupSessionListCursorResponse{List: rspList, NextCursor: next, HasMore: more}, nil
}

func (s *GrpcServer) DeleteSession(ctx context.Context, req *messagepb.DeleteSessionRequest) (*messagepb.DeleteSessionResponse, error) {
	err := s.sessionSvc.DeleteSession(ctx, req.OwnerId, req.SessionId)
	return &messagepb.DeleteSessionResponse{}, err
}

func (s *GrpcServer) PinSession(ctx context.Context, req *messagepb.PinSessionRequest) (*messagepb.PinSessionResponse, error) {
	err := s.sessionSvc.PinSession(ctx, req.UserId, req.SessionId, req.IsPinned)
	return &messagepb.PinSessionResponse{}, err
}

func (s *GrpcServer) DeleteGroupMemberSessions(ctx context.Context, req *messagepb.DeleteGroupMemberSessionsRequest) (*messagepb.DeleteGroupMemberSessionsResponse, error) {
	err := s.sessionSvc.DeleteGroupMemberSessions(ctx, req.GroupId, req.UserIds)
	return &messagepb.DeleteGroupMemberSessionsResponse{}, err
}

func (s *GrpcServer) CreateGroupSession(ctx context.Context, req *messagepb.CreateGroupSessionRequest) (*messagepb.CreateGroupSessionResponse, error) {
	sid, err := s.sessionSvc.CreateGroupSession(ctx, req.GroupId, req.UserId, req.GroupName, req.GroupAvatar)
	if err != nil {
		return nil, err
	}
	return &messagepb.CreateGroupSessionResponse{SessionId: sid}, nil
}

func (s *GrpcServer) DeleteFriendSessions(ctx context.Context, req *messagepb.DeleteFriendSessionsRequest) (*messagepb.DeleteFriendSessionsResponse, error) {
	err := s.sessionSvc.DeleteFriendSessions(ctx, req.UserOneId, req.UserTwoId)
	return &messagepb.DeleteFriendSessionsResponse{}, err
}

