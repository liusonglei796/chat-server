package message

import (
	"context"
	"strconv"
	"strings"

	"github.com/go-kratos/kratos/v2/transport"

	messagepb "kama_chat_server/api/gen/message"
	"kama_chat_server/internal/apps/message/session"
	messagereq "kama_chat_server/internal/common/dto/request/message"
	sessionreq "kama_chat_server/internal/common/dto/request/session"
	"kama_chat_server/pkg/errorx"
	"kama_chat_server/pkg/interceptor"
)

// extractTrustedUserID 从 Kratos HTTP 传输上下文或标准 Context 中提取网关注入的受信用户身份
// 来源: APISIX/网关透传的 X-User-Id 请求头或上下文键值
// 用于: 会话查询、消息发送与撤回操作的身份校验与绑定
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

// ==================== 1. 会话权限检查与创建 ====================

func (s *GrpcServer) CheckOpenSessionAllowed(ctx context.Context, req *messagepb.CheckOpenSessionAllowedRequest) (*messagepb.CheckOpenSessionAllowedResponse, error) {
	// Step 1.1: 确认会话检查的发起人与接收人身份
	// 来源: req.SendId (或网关 X-User-Id) 及 req.ReceiveId (或 req.TargetId)
	// 用于: Step 1.2 调用领域层校验会话权限
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

	// Step 1.2: 执行领域权限校验
	// 来源: Step 1.1 校验后的 sendId 和 receiveId
	// 用于: 返回是否允许建立会话
	allowed, err := s.sessionSvc.CheckOpenSessionAllowed(ctx, sendId, receiveId)
	return &messagepb.CheckOpenSessionAllowedResponse{Allowed: allowed}, err
}

func (s *GrpcServer) OpenSession(ctx context.Context, req *messagepb.OpenSessionRequest) (*messagepb.OpenSessionResponse, error) {
	// Step 1.3: 确认会话发起人身份并校验入参
	// 来源: req.SendId (或网关 X-User-Id) 及 req.ReceiveId
	// 用于: Step 1.4 调用领域层打开/创建会话
	sendId := req.SendId
	if sendId == "" {
		sendId = extractTrustedUserID(ctx)
	}
	if sendId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 1.4: 校验会话权限并创建/获取会话
	// 来源: Step 1.3 的 sendId 及 req.ReceiveId
	// 用于: 返回会话唯一标识 SessionId
	allowed, err := s.sessionSvc.CheckOpenSessionAllowed(ctx, sendId, req.ReceiveId)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errorx.New(errorx.CodeForbidden, "不允许发起会话")
	}

	sid, err := s.sessionSvc.OpenSession(ctx, sendId, sessionreq.OpenSessionRequest{
		ReceiveId:   req.ReceiveId,
		ClientToken: req.ClientToken,
	})
	return &messagepb.OpenSessionResponse{SessionId: sid}, err
}

func (s *GrpcServer) CreateSession(ctx context.Context, req *messagepb.CreateSessionRequest) (*messagepb.CreateSessionResponse, error) {
	// Step 1.5: 内部调用创建单聊会话
	// 来源: req.SendId 及 req.ReceiveId
	// 用于: 返回生成的 session_id
	sid, err := s.sessionSvc.CreateSession(ctx, req.SendId, req.ReceiveId)
	return &messagepb.CreateSessionResponse{SessionId: sid}, err
}

func (s *GrpcServer) CreateGroupSession(ctx context.Context, req *messagepb.CreateGroupSessionRequest) (*messagepb.CreateGroupSessionResponse, error) {
	// Step 1.6: 内部调用创建群聊会话
	// 来源: 群组创建或加群时入参 req
	// 用于: 返回生成的 group session_id
	sid, err := s.sessionSvc.CreateGroupSession(ctx, req.GroupId, req.UserId, req.GroupName, req.GroupAvatar)
	if err != nil {
		return nil, err
	}
	return &messagepb.CreateGroupSessionResponse{SessionId: sid}, nil
}

// ==================== 2. 会话列表查询与管理 ====================

func (s *GrpcServer) GetUserSessionListCursor(ctx context.Context, req *messagepb.GetUserSessionListCursorRequest) (*messagepb.GetUserSessionListCursorResponse, error) {
	// Step 2.1: 确认单聊会话列表所有者身份
	// 来源: req.OwnerId 或网关 X-User-Id
	// 用于: Step 2.2 调用游标分页拉取单聊会话列表
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

	// Step 2.2: 检索单聊会话列表
	// 来源: Step 2.1 确认的 ownerId, req.Cursor 与 pageSize
	// 用于: Step 2.3 组装返回会话与下一游标
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
	// Step 2.3: 确认群聊会话列表所有者身份
	// 来源: req.OwnerId 或网关 X-User-Id
	// 用于: Step 2.4 调用游标分页拉取群聊会话列表
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

	// Step 2.4: 检索群聊会话列表
	// 来源: Step 2.3 确认的 ownerId, req.Cursor 与 pageSize
	// 用于: Step 2.5 组装返回群会话与下一游标
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
	// Step 2.6: 确认所有者身份并传统页码拉取单聊会话
	// 来源: req.OwnerId 或网关 X-User-Id
	// 用于: 返回传统分页会话列表
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
	// Step 2.7: 确认所有者身份并传统页码拉取群聊会话
	// 来源: req.OwnerId 或网关 X-User-Id
	// 用于: 返回传统分页群会话列表
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
	// Step 2.8: 确认会话置顶操作者身份
	// 来源: req.UserId 或网关 X-User-Id
	// 用于: Step 2.9 执行置顶/取消置顶
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 2.9: 调用领域层更新会话置顶状态
	// 来源: Step 2.8 的 userId 及 req.SessionId, req.IsPinned
	// 用于: 返回操作成功响应
	err := s.sessionSvc.PinSession(ctx, userId, req.SessionId, req.IsPinned)
	return &messagepb.PinSessionResponse{}, err
}

func (s *GrpcServer) DeleteSession(ctx context.Context, req *messagepb.DeleteSessionRequest) (*messagepb.DeleteSessionResponse, error) {
	// Step 2.10: 确认会话删除操作者身份
	// 来源: req.OwnerId 或网关 X-User-Id
	// 用于: Step 2.11 执行单端会话隐藏/删除
	ownerId := req.OwnerId
	if ownerId == "" {
		ownerId = extractTrustedUserID(ctx)
	}
	if ownerId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 2.11: 调用领域层删除会话
	// 来源: Step 2.10 的 ownerId 与 req.SessionId
	// 用于: 返回删除结果
	err := s.sessionSvc.DeleteSession(ctx, ownerId, req.SessionId)
	return &messagepb.DeleteSessionResponse{}, err
}

func (s *GrpcServer) DeleteGroupMemberSessions(ctx context.Context, req *messagepb.DeleteGroupMemberSessionsRequest) (*messagepb.DeleteGroupMemberSessionsResponse, error) {
	// Step 2.12: 群成员变动时批量清理成员会话
	// 来源: req.GroupId 与 req.UserIds
	// 用于: 返回清理结果
	err := s.sessionSvc.DeleteGroupMemberSessions(ctx, req.GroupId, req.UserIds)
	return &messagepb.DeleteGroupMemberSessionsResponse{}, err
}

func (s *GrpcServer) DeleteFriendSessions(ctx context.Context, req *messagepb.DeleteFriendSessionsRequest) (*messagepb.DeleteFriendSessionsResponse, error) {
	// Step 2.13: 好友关系解除时双向清理单聊会话
	// 来源: req.UserOneId 与 req.UserTwoId
	// 用于: 返回清理结果
	err := s.sessionSvc.DeleteFriendSessions(ctx, req.UserOneId, req.UserTwoId)
	return &messagepb.DeleteFriendSessionsResponse{}, err
}

// ==================== 3. 消息发送、查询与撤回 ====================

func (s *GrpcServer) SendMessage(ctx context.Context, req *messagepb.SendMessageRequest) (*messagepb.SendMessageResponse, error) {
	// Step 3.1: 接收并转换消息发送实体，双通道提取防重凭证
	// 来源: req (来自客户端或网关) 与 gRPC Metadata
	// 用于: Step 3.2 领域层执行持久化、广播与推送
	clientMsgId := req.ClientMsgId
	if clientMsgId == "" {
		clientMsgId = interceptor.GetIdempotencyKeyFromContext(ctx)
	}

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
		ClientMsgId: clientMsgId,
	}

	// Step 3.2: 调用消息领域层发送处理
	// 来源: Step 3.1 组装的 chatReq
	// 用于: 返回消息 UUID 与创建时间
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
	// Step 3.3: 确认消息查询发起人身份
	// 来源: req.RequesterId 或网关 X-User-Id
	// 用于: Step 3.4 依据 target_id 类型分流查询
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

	// Step 3.4: 根据目标 ID 前缀分流至群聊游标或私聊游标接口
	// 来源: Step 3.3 的 userId 与 req.TargetId
	// 用于: 返回统一的消息历史列表
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
	// Step 3.5: 确认私聊消息查询发起人身份
	// 来源: req.RequesterId 或网关 X-User-Id
	// 用于: Step 3.6 游标拉取单聊消息
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

	// Step 3.6: 调用领域层拉取单聊游标历史
	// 来源: Step 3.5 的 requesterId, req.PartnerId, req.Cursor 与 pageSize
	// 用于: Step 3.7 格式化返回消息列表与游标
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
	// Step 3.8: 确认群聊消息查询用户身份
	// 来源: req.UserId 或网关 X-User-Id
	// 用于: Step 3.9 游标拉取群聊消息
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

	// Step 3.9: 调用领域层拉取群聊游标历史
	// 来源: Step 3.8 的 userId, req.GroupId, req.Cursor 与 pageSize
	// 用于: Step 3.10 格式化返回群消息列表与游标
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
	// Step 3.11: 传统分页拉取私聊消息
	// 来源: req.RequesterId, req.PartnerId, req.Page, req.PageSize
	// 用于: 组装返回历史消息及总量
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
	// Step 3.12: 传统分页拉取群聊消息
	// 来源: req.UserId, req.GroupId, req.Page, req.PageSize
	// 用于: 组装返回群消息历史及总量
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
	// Step 3.13: 确认消息撤回发起人身份
	// 来源: req.UserId 或网关 X-User-Id
	// 用于: Step 3.14 执行消息撤回
	userId := req.UserId
	if userId == "" {
		userId = extractTrustedUserID(ctx)
	}
	if userId == "" {
		return nil, errorx.New(errorx.CodeUnauthorized, "请先登录")
	}

	// Step 3.14: 调用领域层撤回消息
	// 来源: Step 3.13 的 userId 与 req.MessageUuid
	// 用于: 返回撤回响应
	err := s.msgSvc.RecallMessage(ctx, userId, messagereq.RecallMessageRequest{MessageUuid: req.MessageUuid})
	return &messagepb.RecallMessageResponse{}, err
}
