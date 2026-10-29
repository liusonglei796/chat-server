package idempotency

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"kama_chat_server/pkg/errorx"
)

type mockIdempotencyStore struct {
	records map[string]*Record
}

func newMockIdempotencyStore() *mockIdempotencyStore {
	return &mockIdempotencyStore{records: make(map[string]*Record)}
}

func (m *mockIdempotencyStore) key(u, op, k string) string {
	return u + ":" + op + ":" + k
}

func (m *mockIdempotencyStore) Begin(ctx context.Context, userID, operation, key string) (*Record, error) {
	compositeKey := m.key(userID, operation, key)
	if r, exists := m.records[compositeKey]; exists {
		if r.Status == StatusProcessing {
			return nil, ErrProcessing
		}
		return r, nil
	}
	r := &Record{
		ID:             uint(len(m.records) + 1),
		UserID:         userID,
		Operation:      operation,
		IdempotencyKey: key,
		Status:         StatusProcessing,
	}
	m.records[compositeKey] = r
	return r, nil
}

func (m *mockIdempotencyStore) Complete(ctx context.Context, recordID uint, response []byte) error {
	for _, r := range m.records {
		if r.ID == recordID {
			r.Status = StatusSuccess
			r.Response = response
			return nil
		}
	}
	return nil
}

func (m *mockIdempotencyStore) Fail(ctx context.Context, recordID uint, err error) error {
	for _, r := range m.records {
		if r.ID == recordID {
			r.Status = StatusFail
			retryable, code, _ := isRetryable(err)
			r.Retryable = retryable
			r.ErrorCode = code
			return nil
		}
	}
	return nil
}

type sampleResponse struct {
	GroupID string `json:"group_id"`
}

func TestExecuteJSON(t *testing.T) {
	store := newMockIdempotencyStore()
	ctx := context.Background()

	t.Run("first execution succeeds and completes record", func(t *testing.T) {
		called := 0
		res, isReplay, err := ExecuteJSON(ctx, store, "U1001", "CreateGroup", "key-1", func(execCtx context.Context) (*sampleResponse, error) {
			called++
			return &sampleResponse{GroupID: "G999"}, nil
		})
		assert.NoError(t, err)
		assert.False(t, isReplay)
		assert.Equal(t, "G999", res.GroupID)
		assert.Equal(t, 1, called)
	})

	t.Run("replay execution returns cached response without calling fn", func(t *testing.T) {
		called := 0
		res, isReplay, err := ExecuteJSON(ctx, store, "U1001", "CreateGroup", "key-1", func(execCtx context.Context) (*sampleResponse, error) {
			called++
			return &sampleResponse{GroupID: "G_SHOULD_NOT_EXECUTE"}, nil
		})
		assert.NoError(t, err)
		assert.True(t, isReplay)
		assert.Equal(t, "G999", res.GroupID)
		assert.Equal(t, 0, called) // fn was not called
	})

	t.Run("in progress execution returns 429", func(t *testing.T) {
		store.records["U1001:CreateGroup:key-in-progress"] = &Record{
			ID:     99,
			Status: StatusProcessing,
		}
		called := 0
		res, isReplay, err := ExecuteJSON(ctx, store, "U1001", "CreateGroup", "key-in-progress", func(execCtx context.Context) (*sampleResponse, error) {
			called++
			return &sampleResponse{GroupID: "G_FAIL"}, nil
		})
		assert.Error(t, err)
		assert.False(t, isReplay)
		assert.Nil(t, res)
		assert.Equal(t, 0, called)

		var codeErr *errorx.CodeError
		assert.ErrorAs(t, err, &codeErr)
		assert.Equal(t, errorx.CodeTooManyRequests, codeErr.Code)
	})

	t.Run("unretryable failure prevents retry", func(t *testing.T) {
		store.records["U1001:CreateGroup:key-failed"] = &Record{
			ID:        100,
			Status:    StatusFail,
			Retryable: false,
			ErrorCode: errorx.CodeInvalidParam,
		}
		called := 0
		res, isReplay, err := ExecuteJSON(ctx, store, "U1001", "CreateGroup", "key-failed", func(execCtx context.Context) (*sampleResponse, error) {
			called++
			return &sampleResponse{GroupID: "G_FAIL"}, nil
		})
		assert.Error(t, err)
		assert.False(t, isReplay)
		assert.Nil(t, res)
		assert.Equal(t, 0, called)
	})
}
