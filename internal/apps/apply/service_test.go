package apply

import (
	"context"
	"testing"
	"time"

	"kama_chat_server/internal/common/domain/store"
	"kama_chat_server/internal/common/model"
	"kama_chat_server/pkg/enum/apply/apply_status"
	"kama_chat_server/pkg/errorx"
)

type fakeApplyStore struct {
	store.ApplyStore
	applies map[string]*model.Apply // key: applicantId + ":" + targetId
	byUuid  map[string]*model.Apply
}

func (f *fakeApplyStore) FindByApplicantIdAndTargetId(_ context.Context, applicantId, targetId string) (*model.Apply, error) {
	key := applicantId + ":" + targetId
	if a, ok := f.applies[key]; ok {
		cpy := *a
		return &cpy, nil
	}
	return nil, errorx.New(errorx.CodeNotFound, "申请不存在")
}

func (f *fakeApplyStore) UpdateStatusIfPending(_ context.Context, uuid string, toStatus int8) (bool, error) {
	a, ok := f.byUuid[uuid]
	if !ok {
		return false, nil
	}
	if a.Status != apply_status.PENDING {
		return false, nil
	}
	a.Status = toStatus
	return true, nil
}

type fakeApplyUOW struct {
	applyStore *fakeApplyStore
}

func (f *fakeApplyUOW) ApplyStore() store.ApplyStore {
	return f.applyStore
}

func (f *fakeApplyUOW) RecordEvent(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (f *fakeApplyUOW) WithTx(fn func(tx any) error) error {
	return fn(f)
}

type fakeCacheService struct {
	store.AsyncCacheService
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

	setupService := func(initialStatus int8) (*ApplyService, *model.Apply) {
		apply := &model.Apply{
			Uuid:        "APPLY_1",
			ApplicantId: "USER_A",
			TargetId:    "USER_B",
			Status:      initialStatus,
		}
		aStore := &fakeApplyStore{
			applies: map[string]*model.Apply{
				"USER_A:USER_B": apply,
			},
			byUuid: map[string]*model.Apply{
				"APPLY_1": apply,
			},
		}
		uow := &fakeApplyUOW{applyStore: aStore}
		svc := NewApplyService(uow, &fakeCacheService{})
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

	t.Run("fails when status is already processed", func(t *testing.T) {
		svc, _ := setupService(apply_status.AGREE)
		err := svc.RefuseFriendApply(ctx, "USER_B", "USER_A")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errorx.IsCode(err, errorx.CodeInvalidParam) {
			t.Errorf("expected CodeInvalidParam (1001), got code: %d, err: %v", errorx.GetCode(err), err)
		}
	})
}
