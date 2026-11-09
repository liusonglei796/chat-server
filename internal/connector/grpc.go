package connector

import (
	"context"
	"time"

	connectorpb "kama_chat_server/api/gen/connector"
	"go.uber.org/zap"
)

// ConnectorGrpcServer 连接服务内部 gRPC 服务实现
type ConnectorGrpcServer struct {
	connectorpb.UnimplementedConnectorServiceServer
	hub *ConnHub
}

// NewConnectorGrpcServer 创建连接服务内部 gRPC 服务实例
func NewConnectorGrpcServer(hub *ConnHub) *ConnectorGrpcServer {
	return &ConnectorGrpcServer{
		hub: hub,
	}
}

// PushMessage 单用户精准直推
func (s *ConnectorGrpcServer) PushMessage(ctx context.Context, req *connectorpb.PushMessageRequest) (*connectorpb.PushMessageResponse, error) {
	if req.TargetUserId == "" {
		return &connectorpb.PushMessageResponse{
			Success:  false,
			ErrorMsg: "target_user_id is empty",
		}, nil
	}

	client := s.hub.GetClient(req.TargetUserId)
	if client == nil {
		return &connectorpb.PushMessageResponse{
			Success:  false,
			ErrorMsg: "user not online on this connector",
		}, nil
	}

	client.SafeSend(req.Payload, req.MessageUuid)
	return &connectorpb.PushMessageResponse{
		Success: true,
	}, nil
}

// BatchPushMessage 批量推送给同一连接服务节点上的用户
func (s *ConnectorGrpcServer) BatchPushMessage(ctx context.Context, req *connectorpb.BatchPushMessageRequest) (*connectorpb.BatchPushMessageResponse, error) {
	var failedUserIds []string
	for _, uid := range req.TargetUserIds {
		client := s.hub.GetClient(uid)
		if client == nil {
			failedUserIds = append(failedUserIds, uid)
			continue
		}

		if client.ctx != nil && client.ctx.Err() != nil {
			failedUserIds = append(failedUserIds, uid)
			continue
		}

		select {
		case client.sendQueue <- req.Payload:
			client.lastActive.Store(time.Now().Unix())
		default:
			zap.L().Warn("user conn send queue full (backpressure), dropping message",
				zap.String("userId", uid),
				zap.String("msgUuid", req.MessageUuid),
			)
			failedUserIds = append(failedUserIds, uid)
		}
	}
	return &connectorpb.BatchPushMessageResponse{
		FailedUserIds: failedUserIds,
	}, nil
}
