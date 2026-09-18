package grpc_server

import (
	"context"

	gatewaypb "kama_chat_server/api/gen/gateway"
	"kama_chat_server/internal/apps/message/chat"
)

// GatewayGrpcServer 网关内部 gRPC 服务实现
type GatewayGrpcServer struct {
	gatewaypb.UnimplementedGatewayServiceServer
	hub *chat.ClientHub
}

// NewGatewayGrpcServer 创建网关内部 gRPC 服务实例
func NewGatewayGrpcServer(hub *chat.ClientHub) *GatewayGrpcServer {
	return &GatewayGrpcServer{
		hub: hub,
	}
}

// PushMessage 单用户精准直推
func (s *GatewayGrpcServer) PushMessage(ctx context.Context, req *gatewaypb.PushMessageRequest) (*gatewaypb.PushMessageResponse, error) {
	if req.TargetUserId == "" {
		return &gatewaypb.PushMessageResponse{
			Success:  false,
			ErrorMsg: "target_user_id is empty",
		}, nil
	}

	client := s.hub.GetClient(req.TargetUserId)
	if client == nil {
		return &gatewaypb.PushMessageResponse{
			Success:  false,
			ErrorMsg: "user not online on this gateway",
		}, nil
	}

	client.SafeSend(req.Payload, req.MessageUuid)
	return &gatewaypb.PushMessageResponse{
		Success: true,
	}, nil
}

// BatchPushMessage 批量推送给同一网关上的用户
func (s *GatewayGrpcServer) BatchPushMessage(ctx context.Context, req *gatewaypb.BatchPushMessageRequest) (*gatewaypb.BatchPushMessageResponse, error) {
	var failedUserIds []string
	for _, uid := range req.TargetUserIds {
		client := s.hub.GetClient(uid)
		if client == nil {
			failedUserIds = append(failedUserIds, uid)
			continue
		}
		client.SafeSend(req.Payload, req.MessageUuid)
	}
	return &gatewaypb.BatchPushMessageResponse{
		FailedUserIds: failedUserIds,
	}, nil
}
