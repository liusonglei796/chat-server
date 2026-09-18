package cdc

import (
	"context"
	"testing"

	"github.com/go-mysql-org/go-mysql/canal"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/schema"
	"github.com/twmb/franz-go/pkg/kgo"

	"kama_chat_server/internal/common/infrastructure/outbox"
)

func TestOutboxEventHandler_OnRow(t *testing.T) {
	var publishedKey string
	var publishedPayload string
	var publishedEventType string
	callCount := 0

	mockPublish := func(ctx context.Context, key []byte, payload []byte, headers []kgo.RecordHeader) error {
		callCount++
		publishedKey = string(key)
		publishedPayload = string(payload)
		for _, h := range headers {
			if h.Key == outbox.EventTypeHeader {
				publishedEventType = string(h.Value)
			}
		}
		return nil
	}

	posStore := NewMemoryPositionStore()
	handler := NewOutboxEventHandler(posStore, mockPublish)

	testTable := &schema.Table{
		Schema: "chat_group",
		Name:   "outbox",
		Columns: []schema.TableColumn{
			{Name: "uuid"},
			{Name: "event_type"},
			{Name: "payload"},
			{Name: "status"},
		},
	}

	t.Run("insert action on outbox table should trigger kafka publish", func(t *testing.T) {
		callCount = 0
		event := &canal.RowsEvent{
			Table:  testTable,
			Action: canal.InsertAction,
			Rows: [][]interface{}{
				{"O1234567890", "group_created", `{"group_id":"G1001","group_name":"Golang Group"}`, 0},
			},
		}

		err := handler.OnRow(event)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 1 {
			t.Fatalf("expected 1 publish call, got %d", callCount)
		}
		if publishedKey != "O1234567890" {
			t.Errorf("expected key O1234567890, got %s", publishedKey)
		}
		if publishedEventType != "group_created" {
			t.Errorf("expected event_type group_created, got %s", publishedEventType)
		}
		if publishedPayload != `{"group_id":"G1001","group_name":"Golang Group"}` {
			t.Errorf("expected payload %s, got %s", `{"group_id":"G1001","group_name":"Golang Group"}`, publishedPayload)
		}
	})

	t.Run("update or delete action should be ignored", func(t *testing.T) {
		callCount = 0
		updateEvent := &canal.RowsEvent{
			Table:  testTable,
			Action: canal.UpdateAction,
			Rows: [][]interface{}{
				{"O1234567890", "group_created", `{}`, 1},
			},
		}
		if err := handler.OnRow(updateEvent); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 0 {
			t.Errorf("expected 0 publish calls for update, got %d", callCount)
		}

		deleteEvent := &canal.RowsEvent{
			Table:  testTable,
			Action: canal.DeleteAction,
			Rows: [][]interface{}{
				{"O1234567890", "group_created", `{}`, 1},
			},
		}
		if err := handler.OnRow(deleteEvent); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 0 {
			t.Errorf("expected 0 publish calls for delete, got %d", callCount)
		}
	})

	t.Run("other tables should be ignored", func(t *testing.T) {
		callCount = 0
		otherTable := &schema.Table{
			Schema: "chat_group",
			Name:   "group_info",
			Columns: []schema.TableColumn{
				{Name: "group_id"},
				{Name: "group_name"},
			},
		}
		otherEvent := &canal.RowsEvent{
			Table:  otherTable,
			Action: canal.InsertAction,
			Rows: [][]interface{}{
				{"G1001", "Golang Group"},
			},
		}
		if err := handler.OnRow(otherEvent); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 0 {
			t.Errorf("expected 0 publish calls for non-outbox table, got %d", callCount)
		}
	})
}

func TestMemoryPositionStore(t *testing.T) {
	store := NewMemoryPositionStore()
	ctx := context.Background()

	pos, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pos.Name != "" || pos.Pos != 0 {
		t.Fatalf("expected empty position initially, got %+v", pos)
	}

	expected := mysql.Position{Name: "mysql-bin.000001", Pos: 12345}
	if err := store.Save(ctx, expected); err != nil {
		t.Fatalf("save error: %v", err)
	}

	actual, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if actual != expected {
		t.Fatalf("expected position %+v, got %+v", expected, actual)
	}
}
