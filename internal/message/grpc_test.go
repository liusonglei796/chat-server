package message

import (
	"context"
	"testing"

	messagepb "kama_chat_server/api/gen/message"
)

func TestGrpcGetMessages_AfterSeq(t *testing.T) {
	msgStore := &mockMsgStore{
		syncMessages: []Message{
			{
				Uuid:      "M1001",
				SessionId: "P_U1001_U1002",
				SendId:    "U1001",
				ReceiveId: "U1002",
				Content:   "hello seq 1",
				SeqId:     1,
			},
			{
				Uuid:      "M1002",
				SessionId: "P_U1001_U1002",
				SendId:    "U1002",
				ReceiveId: "U1001",
				Content:   "world seq 2",
				SeqId:     2,
			},
		},
	}
	sessStore := &mockSessStore{}
	cache := newMockCache()
	msgSvc := NewMessageService(msgStore, sessStore, cache)
	sessSvc := NewSessionService(sessStore, msgStore, cache)
	server := NewGrpcServer(msgSvc, sessSvc)

	ctx := context.Background()

	t.Run("Sync after_seq:0 for private chat", func(t *testing.T) {
		resp, err := server.GetMessages(ctx, &messagepb.GetMessagesRequest{
			RequesterId: "U1001",
			TargetId:    "U1002",
			Cursor:      "after_seq:0",
			PageSize:    10,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.List) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(resp.List))
		}
		if resp.List[0].SeqId != 1 || resp.List[1].SeqId != 2 {
			t.Fatalf("expected seqIds 1 and 2, got %d and %d", resp.List[0].SeqId, resp.List[1].SeqId)
		}
		if resp.NextCursor != "after_seq:2" {
			t.Fatalf("expected NextCursor 'after_seq:2', got %q", resp.NextCursor)
		}
		if resp.HasMore {
			t.Fatalf("expected HasMore to be false, got true")
		}
	})

	t.Run("Sync after_seq with empty list maintains cursor", func(t *testing.T) {
		emptyStore := &mockMsgStore{}
		emptyMsgSvc := NewMessageService(emptyStore, sessStore, cache)
		emptyServer := NewGrpcServer(emptyMsgSvc, sessSvc)

		resp, err := emptyServer.GetMessages(ctx, &messagepb.GetMessagesRequest{
			RequesterId: "U1001",
			TargetId:    "U1002",
			Cursor:      "after_seq:5",
			PageSize:    10,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(resp.List) != 0 {
			t.Fatalf("expected 0 messages, got %d", len(resp.List))
		}
		if resp.NextCursor != "after_seq:5" {
			t.Fatalf("expected NextCursor 'after_seq:5', got %q", resp.NextCursor)
		}
		if resp.HasMore {
			t.Fatalf("expected HasMore to be false")
		}
	})
}
