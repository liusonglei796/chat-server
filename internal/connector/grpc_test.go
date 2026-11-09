package connector

import (
	"context"
	"testing"

	connectorpb "kama_chat_server/api/gen/connector"
)

func TestConnectorGrpcServer_PushMessage_Offline(t *testing.T) {
	hub := NewConnHub(nil, "127.0.0.1:8001")
	server := NewConnectorGrpcServer(hub)

	resp, err := server.PushMessage(context.Background(), &connectorpb.PushMessageRequest{
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

func TestConnectorGrpcServer_PushMessage_EmptyTarget(t *testing.T) {
	hub := NewConnHub(nil, "127.0.0.1:8001")
	server := NewConnectorGrpcServer(hub)

	resp, err := server.PushMessage(context.Background(), &connectorpb.PushMessageRequest{
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

func TestConnectorGrpcServer_PushMessage_Online(t *testing.T) {
	hub := NewConnHub(nil, "127.0.0.1:8001")
	server := NewConnectorGrpcServer(hub)

	client := &UserConn{
		Uuid:      "U_online",
		sendQueue: make(chan []byte, 10),
	}
	hub.Clients.Store(client.Uuid, client)

	resp, err := server.PushMessage(context.Background(), &connectorpb.PushMessageRequest{
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
	case msg := <-client.sendQueue:
		if string(msg) != `{"test":"msg"}` {
			t.Fatalf("unexpected payload: %s", string(msg))
		}
	default:
		t.Fatalf("expected message in sendQueue channel")
	}
}

func TestConnectorGrpcServer_BatchPushMessage(t *testing.T) {
	hub := NewConnHub(nil, "127.0.0.1:8001")
	server := NewConnectorGrpcServer(hub)

	client := &UserConn{
		Uuid:      "U_user1",
		sendQueue: make(chan []byte, 10),
	}
	hub.Clients.Store(client.Uuid, client)

	resp, err := server.BatchPushMessage(context.Background(), &connectorpb.BatchPushMessageRequest{
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
