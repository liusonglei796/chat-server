package message

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-kratos/kratos/v2/transport"

	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
)

// extractTrustedUserID 从 Kratos HTTP 传输上下文或标准 Context 中提取网关注入的受信用户身份
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

// GrpcServer gRPC 服务端适配器，聚合消息与会话领域服务
type GrpcServer struct {
	messagepb.UnimplementedMessageServiceServer
	msgSvc     *MessageService
	sessionSvc *SessionService
}

// NewGrpcServer 创建 GrpcServer 实例
func NewGrpcServer(msgSvc *MessageService, sessionSvc *SessionService) *GrpcServer {
	return &GrpcServer{
		msgSvc:     msgSvc,
		sessionSvc: sessionSvc,
	}
}

// ==================== 1. 会话权限检查与创建 ====================

func (s *GrpcServer) CheckOpenSessionAllowed(ctx context.Context, req *messagepb.CheckOpenSessionAllowedRequest) (*messagepb.CheckOpenSessionAllowedResponse, error) {
	sendId := req.SendId
	if sendId == "" {
		sendId = extractTrustedUserID(ctx)
	}
	if sendId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	receiveId := req.ReceiveId
	if receiveId == "" {
		receiveId = req.TargetId
	}
	if receiveId == "" {
		return nil, errorx.New(errorx.CodeInvalidParam, "目标ID不能为空")
	}

	allowed, err := s.sessionSvc.CheckOpenSessionAllowed(ctx, sendId, receiveId)
	return &messagepb.CheckOpenSessionAllowedResponse{Allowed: allowed}, err
}

func (s *GrpcServer) OpenSession(ctx context.Context, req *messagepb.OpenSessionRequest) (*messagepb.OpenSessionResponse, error) {
	sendId := req.SendId
	if sendId == "" {
		sendId = extractTrustedUserID(ctx)
	}
	if sendId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	allowed, err := s.sessionSvc.CheckOpenSessionAllowed(ctx, sendId, req.ReceiveId)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errorx.New(errorx.CodeForbidden, "不允许发起会话")
	}

	sid, err := s.sessionSvc.OpenSession(ctx, sendId, OpenSessionRequest{
		ReceiveId:   req.ReceiveId,
		ClientToken: req.ClientToken,
	})
	return &messagepb.OpenSessionResponse{SessionId: sid}, err
}

func (s *GrpcServer) CreateSession(ctx context.Context, req *messagepb.CreateSessionRequest) (*messagepb.CreateSessionResponse, error) {
	sid, err := s.sessionSvc.CreateSession(ctx, req.SendId, req.ReceiveId)
	return &messagepb.CreateSessionResponse{SessionId: sid}, err
}

func (s *GrpcServer) CreateGroupSession(ctx context.Context, req *messagepb.CreateGroupSessionRequest) (*messagepb.CreateGroupSessionResponse, error) {
	sid, err := s.sessionSvc.CreateGroupSession(ctx, req.GroupId, req.UserId, req.GroupName, req.GroupAvatar)
	if err != nil {
		return nil, err
	}
	return &messagepb.CreateGroupSessionResponse{SessionId: sid}, nil
}

// ==================== 2. 会话列表查询与管理 ====================

func (s *GrpcServer) GetUserSessionListCursor(ctx context.Context, req *messagepb.GetUserSessionListCursorRequest) (*messagepb.GetUserSessionListCursorResponse, error) {
	ownerId := req.OwnerId
	if ownerId == "" {
		ownerId = extractTrustedUserID(ctx)
	}
	if ownerId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	pageSize := int(req.PageSize)
	if pageSize < 1 {
		pageSize = 20
	}

	list, next, more, err := s.sessionSvc.GetUserSessionListCursor(ctx, ownerId, req.Cursor, pageSize)
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
	ownerId := req.OwnerId
	if ownerId == "" {
		ownerId = extractTrustedUserID(ctx)
	}
	if ownerId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	pageSize := int(req.PageSize)
	if pageSize < 1 {
		pageSize = 20
	}

	list, next, more, err := s.sessionSvc.GetGroupSessionListCursor(ctx, ownerId, req.Cursor, pageSize)
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

func (s *GrpcServer) GetUserSessionList(ctx context.Context, req *messagepb.GetUserSessionListRequest) (*messagepb.GetUserSessionListResponse, error) {
	ownerId := req.OwnerId
	if ownerId == "" {
		ownerId = extractTrustedUserID(ctx)
	}
	list, total, err := s.sessionSvc.GetUserSessionList(ctx, ownerId, int(req.Page), int(req.PageSize))
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
	ownerId := req.OwnerId
	if ownerId == "" {
		ownerId = extractTrustedUserID(ctx)
	}
	list, total, err := s.sessionSvc.GetGroupSessionList(ctx, ownerId, int(req.Page), int(req.PageSize))
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

func (s *GrpcServer) PinSession(ctx context.Context, req *messagepb.PinSessionRequest) (*messagepb.PinSessionResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.sessionSvc.PinSession(ctx, userId, req.SessionId, req.IsPinned)
	return &messagepb.PinSessionResponse{}, err
}

func (s *GrpcServer) DeleteSession(ctx context.Context, req *messagepb.DeleteSessionRequest) (*messagepb.DeleteSessionResponse, error) {
	ownerId := req.OwnerId
	if ownerId == "" {
		ownerId = extractTrustedUserID(ctx)
	}
	if ownerId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.sessionSvc.DeleteSession(ctx, ownerId, req.SessionId)
	return &messagepb.DeleteSessionResponse{}, err
}

func (s *GrpcServer) DeleteGroupMemberSessions(ctx context.Context, req *messagepb.DeleteGroupMemberSessionsRequest) (*messagepb.DeleteGroupMemberSessionsResponse, error) {
	err := s.sessionSvc.DeleteGroupMemberSessions(ctx, req.GroupId, req.UserIds)
	return &messagepb.DeleteGroupMemberSessionsResponse{}, err
}

func (s *GrpcServer) DeleteFriendSessions(ctx context.Context, req *messagepb.DeleteFriendSessionsRequest) (*messagepb.DeleteFriendSessionsResponse, error) {
	err := s.sessionSvc.DeleteFriendSessions(ctx, req.UserOneId, req.UserTwoId)
	return &messagepb.DeleteFriendSessionsResponse{}, err
}

// ==================== 3. 消息发送、查询与撤回 ====================

func (s *GrpcServer) SendMessage(ctx context.Context, req *messagepb.SendMessageRequest) (*messagepb.SendMessageResponse, error) {
	clientMsgId := req.ClientMsgId
	if clientMsgId == "" {
		clientMsgId = interceptor.GetIdempotencyKeyFromContext(ctx)
	}

	chatReq := ChatMessageRequest{
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
		ClientMsgId: clientMsgId,
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

func (s *GrpcServer) GetMessages(ctx context.Context, req *messagepb.GetMessagesRequest) (*messagepb.GetMessagesResponse, error) {
	userId := req.RequesterId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	pageSize := req.PageSize
	if pageSize < 1 {
		pageSize = 20
	}

	if strings.HasPrefix(req.TargetId, "G") {
		rsp, err := s.GetGroupMessageListCursor(ctx, &messagepb.GetGroupMessageListCursorRequest{
			UserId:   userId,
			GroupId:  req.TargetId,
			Cursor:   req.Cursor,
			PageSize: pageSize,
		})
		if err != nil {
			return nil, err
		}
		return &messagepb.GetMessagesResponse{
			List:       rsp.List,
			NextCursor: rsp.NextCursor,
			HasMore:    rsp.HasMore,
		}, nil
	}

	rsp, err := s.GetMessageListCursor(ctx, &messagepb.GetMessageListCursorRequest{
		RequesterId: userId,
		PartnerId:   req.TargetId,
		Cursor:      req.Cursor,
		PageSize:    pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &messagepb.GetMessagesResponse{
		List:       rsp.List,
		NextCursor: rsp.NextCursor,
		HasMore:    rsp.HasMore,
	}, nil
}

func (s *GrpcServer) GetMessageListCursor(ctx context.Context, req *messagepb.GetMessageListCursorRequest) (*messagepb.GetMessageListCursorResponse, error) {
	requesterId := req.RequesterId
	if requesterId == "" {
		requesterId = extractTrustedUserID(ctx)
	}
	if requesterId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	pageSize := int(req.PageSize)
	if pageSize < 1 {
		pageSize = 20
	}

	list, next, more, err := s.msgSvc.GetMessageListCursor(ctx, requesterId, req.PartnerId, req.Cursor, pageSize)
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
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	pageSize := int(req.PageSize)
	if pageSize < 1 {
		pageSize = 20
	}

	list, next, more, err := s.msgSvc.GetGroupMessageListCursor(ctx, userId, req.GroupId, req.Cursor, pageSize)
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

func (s *GrpcServer) RecallMessage(ctx context.Context, req *messagepb.RecallMessageRequest) (*messagepb.RecallMessageResponse, error) {
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	err := s.msgSvc.RecallMessage(ctx, userId, RecallMessageRequest{MessageUuid: req.MessageUuid})
	return &messagepb.RecallMessageResponse{}, err
}
