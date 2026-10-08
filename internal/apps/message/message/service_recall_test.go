package message

import (
	"context"
	"testing"
	"time"

	"kama_chat_server/internal/common/domain/store"
	messagereq "kama_chat_server/internal/common/dto/request/message"
	"kama_chat_server/internal/common/model"
	msgtype "kama_chat_server/pkg/enum/message/message_type"
	"kama_chat_server/pkg/errorx"
)

type mockRecallMessageStore struct {
	store.MessageStore
	msg          *model.Message
	updateOk     bool
	updateCalled int
}

func (m *mockRecallMessageStore) FindByUuid(_ context.Context, uuid string) (*model.Message, error) {
	if m.msg != nil && m.msg.Uuid == uuid {
		cpy := *m.msg
		return &cpy, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "消息不存在")
}

func (m *mockRecallMessageStore) UpdateContent(_ context.Context, uuid, content string, msgType int8) (bool, error) {
	m.updateCalled++
	if m.updateOk {
		m.msg.Content = content
		m.msg.Type = msgType
		return true, nil
	}
	return false, nil
}

func TestMessageService_RecallMessage_Atomic(t *testing.T) {
	ctx := context.Background()

	t.Run("first recall succeeds and triggers notify", func(t *testing.T) {
		msgStore := &mockRecallMessageStore{
			msg: &model.Message{
				Uuid:      "M1001",
				SendId:    "U1",
				ReceiveId: "U2",
				Type:      int8(msgtype.Text),
				Content:   "hello",
			},
			updateOk: true,
		}
		msgStore.msg.CreatedAt = time.Now()

		svc := NewMessageService(msgStore, nil, nil)
		notifyCalled := false
		svc.SetPushRecallNotify(func(messageUuid, receiveId string) {
			notifyCalled = true
		})

		err := svc.RecallMessage(ctx, "U1", messagereq.RecallMessageRequest{
			MessageUuid: "M1001",
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !notifyCalled {
			t.Fatal("expected notify to be called")
		}
		if msgStore.updateCalled != 1 {
			t.Fatalf("expected updateCalled=1, got %d", msgStore.updateCalled)
		}
	})

	t.Run("concurrent duplicate recall fails and does not notify", func(t *testing.T) {
		msgStore := &mockRecallMessageStore{
			msg: &model.Message{
				Uuid:      "M1001",
				SendId:    "U1",
				ReceiveId: "U2",
				Type:      int8(msgtype.Text),
				Content:   "hello",
			},
			updateOk: false, // 模拟并发时 RowsAffected == 0（已被其他请求撤回）
		}
		msgStore.msg.CreatedAt = time.Now()

		svc := NewMessageService(msgStore, nil, nil)
		notifyCalled := false
		svc.SetPushRecallNotify(func(messageUuid, receiveId string) {
			notifyCalled = true
		})

		err := svc.RecallMessage(ctx, "U1", messagereq.RecallMessageRequest{
			MessageUuid: "M1001",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errorx.IsCode(err, errorx.CodeInvalidParam) {
			t.Fatalf("expected CodeInvalidParam, got %v", err)
		}
		if notifyCalled {
			t.Fatal("expected notify NOT to be called on duplicate recall")
		}
	})
}
