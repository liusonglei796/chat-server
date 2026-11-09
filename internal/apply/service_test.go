package apply

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	applypb "kama_chat_server/api/gen/apply"
	userpb "kama_chat_server/api/gen/user"
	cacheinfra "kama_chat_server/internal/infra/cache"
	"kama_chat_server/internal/infra/grpc_client"
	"kama_chat_server/pkg/enum/apply/apply_status"
	"kama_chat_server/pkg/errorx"
)

type fakeApplyStore struct {
	Store
	mu      sync.Mutex
	applies map[string]*Apply // key: applicantId + ":" + targetId
	byUuid  map[string]*Apply
}

func (f *fakeApplyStore) FindByApplicantIdAndTargetId(_ context.Context, applicantId, targetId string) (*Apply, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := applicantId + ":" + targetId
	if a, ok := f.applies[key]; ok {
		cpy := *a
		return &cpy, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "申请不存在")
}

func (f *fakeApplyStore) UpdateStatusIfPending(_ context.Context, uuid string, toStatus int8, expectedVersion int32) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.byUuid[uuid]
	if !ok {
		return false, nil
	}
	if a.Status != apply_status.PENDING {
		return false, nil
	}
	if expectedVersion > 0 && a.Version != expectedVersion {
		return false, nil
	}
	a.Status = toStatus
	a.Version++
	return true, nil
}

func (f *fakeApplyStore) UpdateWithVersion(_ context.Context, apply *Apply, expectedVersion int32) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := apply.ApplicantId + ":" + apply.TargetId
	a, ok := f.applies[key]
	if !ok {
		return false, nil
	}
	if expectedVersion > 0 && a.Version != expectedVersion {
		return false, nil
	}
	*a = *apply
	return true, nil
}

type fakeCacheService struct {
	cacheinfra.AsyncCacheService
}

func (f *fakeCacheService) SubmitTask(task func()) {
	task()
}

func (f *fakeCacheService) Delete(_ context.Context, _ string) error {
	return nil
}

func (f *fakeCacheService) DeleteByPattern(_ context.Context, _ ...string) error {
	return nil
}

func (f *fakeCacheService) InvalidateWithNull(_ context.Context, _ string) error {
	return nil
}

func (f *fakeCacheService) Set(_ context.Context, _ string, _ string, _ time.Duration) error {
	return nil
}

func TestApplyService_RefuseFriendApply_AtomicStatus(t *testing.T) {
	ctx := context.Background()

	setupService := func(initialStatus int8) (*ApplyService, *Apply) {
		apply := &Apply{
			Uuid:        "APPLY_1",
			ApplicantId: "USER_A",
			TargetId:    "USER_B",
			Status:      initialStatus,
			Version:     1,
		}
		aStore := &fakeApplyStore{
			applies: map[string]*Apply{
				"USER_A:USER_B": apply,
			},
			byUuid: map[string]*Apply{
				"APPLY_1": apply,
			},
		}
		svc := NewApplyService(nil, aStore, &fakeCacheService{})
		return svc, apply
	}

	t.Run("success when status is pending", func(t *testing.T) {
		svc, apply := setupService(apply_status.PENDING)
		err := svc.RefuseFriendApply(ctx, "USER_B", "USER_A")
		if err != nil {
			t.Fatalf("expected nil err, got: %v", err)
		}
		if apply.Status != apply_status.REFUSE {
			t.Errorf("expected status %d, got %d", apply_status.REFUSE, apply.Status)
		}
	})

	t.Run("fails when status is already processed with different target", func(t *testing.T) {
		svc, _ := setupService(apply_status.AGREE)
		err := svc.RefuseFriendApply(ctx, "USER_B", "USER_A")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !applypb.IsApplyAlreadyProcessed(err) {
			t.Errorf("expected IsApplyAlreadyProcessed, got: %v", err)
		}
	})

	t.Run("heals idempotently when status is already refused", func(t *testing.T) {
		svc, apply := setupService(apply_status.REFUSE)
		err := svc.RefuseFriendApply(ctx, "USER_B", "USER_A")
		if err != nil {
			t.Fatalf("expected nil err for already refused apply, got: %v", err)
		}
		if apply.Status != apply_status.REFUSE {
			t.Errorf("expected status %d, got %d", apply_status.REFUSE, apply.Status)
		}
	})
}

func TestApplyService_BlackFriendApply_IdempotentHealing(t *testing.T) {
	ctx := context.Background()

	setupService := func(initialStatus int8) (*ApplyService, *Apply) {
		apply := &Apply{
			Uuid:        "APPLY_2",
			ApplicantId: "USER_A",
			TargetId:    "USER_B",
			Status:      initialStatus,
			Version:     1,
		}
		aStore := &fakeApplyStore{
			applies: map[string]*Apply{
				"USER_A:USER_B": apply,
			},
			byUuid: map[string]*Apply{
				"APPLY_2": apply,
			},
		}
		svc := NewApplyService(nil, aStore, &fakeCacheService{})
		return svc, apply
	}

	t.Run("first black succeeds", func(t *testing.T) {
		svc, apply := setupService(apply_status.PENDING)
		err := svc.BlackFriendApply(ctx, "USER_B", "USER_A")
		if err != nil {
			t.Fatalf("expected nil err, got: %v", err)
		}
		if apply.Status != apply_status.BLACK {
			t.Errorf("expected status %d, got %d", apply_status.BLACK, apply.Status)
		}
	})

	t.Run("retry black heals idempotently when already blacked", func(t *testing.T) {
		svc, apply := setupService(apply_status.BLACK)
		err := svc.BlackFriendApply(ctx, "USER_B", "USER_A")
		if err != nil {
			t.Fatalf("expected nil err for already blacked apply, got: %v", err)
		}
		if apply.Status != apply_status.BLACK {
			t.Errorf("expected status %d, got %d", apply_status.BLACK, apply.Status)
		}
	})
}

type mockUserServiceClient struct {
	userpb.UserServiceClient
}

func (m *mockUserServiceClient) GetUserStatus(_ context.Context, _ *userpb.GetUserStatusRequest, _ ...grpc.CallOption) (*userpb.GetUserStatusResponse, error) {
	return &userpb.GetUserStatusResponse{Status: 0}, nil
}

func TestApplyService_PassFriendApply_Concurrent(t *testing.T) {
	ctx := context.Background()
	oldClient := grpc_client.UserClient
	grpc_client.UserClient = &mockUserServiceClient{}
	defer func() {
		grpc_client.UserClient = oldClient
	}()

	apply := &Apply{
		Uuid:        "APPLY_CONC",
		ApplicantId: "USER_A",
		TargetId:    "USER_B",
		Status:      apply_status.PENDING,
		Version:     1,
	}
	aStore := &fakeApplyStore{
		applies: map[string]*Apply{
			"USER_A:USER_B": apply,
		},
		byUuid: map[string]*Apply{
			"APPLY_CONC": apply,
		},
	}

	svc := NewApplyService(nil, aStore, &fakeCacheService{})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = svc.PassFriendApply(ctx, "USER_B", "USER_A")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d returned error: %v", i, err)
		}
	}

	if apply.Status != apply_status.AGREE {
		t.Fatalf("expected apply status AGREE, got %d", apply.Status)
	}
	if apply.Version != 2 {
		t.Fatalf("expected apply version 2, got %d", apply.Version)
	}
}
