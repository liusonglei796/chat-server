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
	var publishedEventId string
	callCount := 0

	mockPublish := func(ctx context.Context, key []byte, payload []byte, headers []kgo.RecordHeader) error {
		callCount++
		publishedKey = string(key)
		publishedPayload = string(payload)
		for _, h := range headers {
			if h.Key == outbox.EventTypeHeader {
				publishedEventType = string(h.Value)
			}
			if h.Key == "event_id" {
				publishedEventId = string(h.Value)
			}
		}
		return nil
	}

	gtidStore := NewMemoryGTIDStore()
	handler := NewOutboxEventHandler(gtidStore, mockPublish)

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

	testTableWithAggregate := &schema.Table{
		Schema: "chat_group",
		Name:   "outbox",
		Columns: []schema.TableColumn{
			{Name: "uuid"},
			{Name: "aggregate_id"},
			{Name: "event_type"},
			{Name: "payload"},
		},
	}

	t.Run("insert action on outbox table should trigger kafka publish with fallback uuid", func(t *testing.T) {
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
		if publishedEventId != "O1234567890" {
			t.Errorf("expected event_id O1234567890, got %s", publishedEventId)
		}
		if publishedPayload != `{"group_id":"G1001","group_name":"Golang Group"}` {
			t.Errorf("expected payload %s, got %s", `{"group_id":"G1001","group_name":"Golang Group"}`, publishedPayload)
		}
	})

	t.Run("insert action on outbox with aggregate_id should route by aggregate_id", func(t *testing.T) {
		callCount = 0
		event := &canal.RowsEvent{
			Table:  testTableWithAggregate,
			Action: canal.InsertAction,
			Rows: [][]interface{}{
				{"O1234567890", "G1001", "group_created", `{"group_id":"G1001","group_name":"Golang Group"}`},
			},
		}

		err := handler.OnRow(event)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 1 {
			t.Fatalf("expected 1 publish call, got %d", callCount)
		}
		if publishedKey != "G1001" {
			t.Errorf("expected key G1001 for partition ordering, got %s", publishedKey)
		}
		if publishedEventId != "O1234567890" {
			t.Errorf("expected event_id O1234567890 in header, got %s", publishedEventId)
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

func TestMemoryGTIDStore(t *testing.T) {
	store := NewMemoryGTIDStore()
	ctx := context.Background()

	set, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if set != nil {
		t.Fatalf("expected nil GTIDSet initially, got %+v", set)
	}

	expected, err := mysql.ParseMysqlGTIDSet("3E11FA47-71CA-11E1-9E33-C80AA9429562:1-5")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if err := store.Save(ctx, expected); err != nil {
		t.Fatalf("save error: %v", err)
	}

	actual, err := store.Get(ctx)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if !actual.Equal(expected) {
		t.Fatalf("expected GTID set %+v, got %+v", expected, actual)
	}
}

func TestOutboxEventHandler_OnPosSynced(t *testing.T) {
	store := NewMemoryGTIDStore()
	handler := NewOutboxEventHandler(store, nil)

	gtidSet, err := mysql.ParseMysqlGTIDSet("3E11FA47-71CA-11E1-9E33-C80AA9429562:1-10")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	err = handler.OnPosSynced(nil, mysql.Position{}, gtidSet, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	saved, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !saved.Equal(gtidSet) {
		t.Fatalf("expected GTID set %+v, got %+v", gtidSet, saved)
	}
}
