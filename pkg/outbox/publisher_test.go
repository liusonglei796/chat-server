package outbox

import (
	"context"
	"errors"
	"testing"

	"kama_chat_server/internal/common/model"
)

type fakeOutboxRepo struct {
	pending   []model.Outbox
	published []string
	retries   map[string]int
}

func (f *fakeOutboxRepo) Create(ctx context.Context, o *model.Outbox) error { return nil }
func (f *fakeOutboxRepo) FindPending(ctx context.Context, limit int) ([]model.Outbox, error) {
	return f.pending, nil
}
func (f *fakeOutboxRepo) MarkPublished(ctx context.Context, uuids []string) error {
	f.published = append(f.published, uuids...)
	return nil
}
func (f *fakeOutboxRepo) IncrementRetry(ctx context.Context, uuid string) error {
	f.retries[uuid]++
	return nil
}

func TestDispatchMarksPublished(t *testing.T) {
	repo := &fakeOutboxRepo{
		pending: []model.Outbox{{Uuid: "E1", EventType: "group_created", Payload: "{}"}},
		retries: map[string]int{},
	}
	p := &Publisher{
		outboxRepo: repo,
		batchSize:  100,
		publishFn: func(ctx context.Context, eventType string, uuid string, payload []byte) error {
			return nil
		},
	}

	if err := p.Dispatch(context.Background()); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(repo.published) != 1 || repo.published[0] != "E1" {
		t.Fatalf("expected E1 published, got %v", repo.published)
	}
	if repo.retries["E1"] != 0 {
		t.Fatalf("expected no retries, got %d", repo.retries["E1"])
	}
}

func TestDispatchFailureIncrementsRetry(t *testing.T) {
	repo := &fakeOutboxRepo{
		pending: []model.Outbox{{Uuid: "E2", EventType: "group_joined", Payload: "{}"}},
		retries: map[string]int{},
	}
	p := &Publisher{
		outboxRepo: repo,
		batchSize:  100,
		publishFn: func(ctx context.Context, eventType string, uuid string, payload []byte) error {
			return errors.New("kafka down")
		},
	}

	if err := p.Dispatch(context.Background()); err != nil {
		t.Fatalf("Dispatch returned error: %v", err)
	}
	if len(repo.published) != 0 {
		t.Fatalf("expected nothing published, got %v", repo.published)
	}
	if repo.retries["E2"] != 1 {
		t.Fatalf("expected retry count 1, got %d", repo.retries["E2"])
	}
}
