package grpc_server

import (
	"context"
	"testing"

	gatewaypb "kama_chat_server/api/gen/gateway"
	"kama_chat_server/internal/apps/message/chat"
)

func TestGatewayGrpcServer_PushMessage_Offline(t *testing.T) {
	hub := chat.NewClientHub(nil, "127.0.0.1:8001")
	server := NewGatewayGrpcServer(hub)

	resp, err := server.PushMessage(context.Background(), &gatewaypb.PushMessageRequest{
		TargetUserId: "U_not_online",
		MessageUuid:  "uuid-123",
		Payload:      []byte("hello"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatalf("expected success=false for offline user, got true")
	}
}

func TestGatewayGrpcServer_PushMessage_EmptyTarget(t *testing.T) {
	hub := chat.NewClientHub(nil, "127.0.0.1:8001")
	server := NewGatewayGrpcServer(hub)

	resp, err := server.PushMessage(context.Background(), &gatewaypb.PushMessageRequest{
		TargetUserId: "",
		MessageUuid:  "uuid-123",
		Payload:      []byte("hello"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatalf("expected success=false for empty target, got true")
	}
}

func TestGatewayGrpcServer_PushMessage_Online(t *testing.T) {
	hub := chat.NewClientHub(nil, "127.0.0.1:8001")
	server := NewGatewayGrpcServer(hub)

	// Simulate client in hub.Clients
	client := &chat.UserConn{
		Uuid:     "U_online",
		SendBack: make(chan *chat.MessageBack, 10),
	}
	hub.Clients.Store(client.Uuid, client)

	resp, err := server.PushMessage(context.Background(), &gatewaypb.PushMessageRequest{
		TargetUserId: "U_online",
		MessageUuid:  "uuid-999",
		Payload:      []byte(`{"test":"msg"}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success=true for online user, got false")
	}

	select {
	case msg := <-client.SendBack:
		if string(msg.Message) != `{"test":"msg"}` {
			t.Fatalf("unexpected payload: %s", string(msg.Message))
		}
		if msg.Uuid != "uuid-999" {
			t.Fatalf("unexpected uuid: %s", msg.Uuid)
		}
	default:
		t.Fatalf("expected message in SendBack channel")
	}
}

func TestGatewayGrpcServer_BatchPushMessage(t *testing.T) {
	hub := chat.NewClientHub(nil, "127.0.0.1:8001")
	server := NewGatewayGrpcServer(hub)

	client := &chat.UserConn{
		Uuid:     "U_user1",
		SendBack: make(chan *chat.MessageBack, 10),
	}
	hub.Clients.Store(client.Uuid, client)

	resp, err := server.BatchPushMessage(context.Background(), &gatewaypb.BatchPushMessageRequest{
		TargetUserIds: []string{"U_user1", "U_user2_offline"},
		MessageUuid:   "uuid-batch",
		Payload:       []byte("batch"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.FailedUserIds) != 1 || resp.FailedUserIds[0] != "U_user2_offline" {
		t.Fatalf("expected failedUserIds=[U_user2_offline], got %v", resp.FailedUserIds)
	}
}
