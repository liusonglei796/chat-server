package apply

import (
	"context"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"kama_chat_server/pkg/enum/apply/apply_status"
)

func TestApplyStore_UpdateStatusIfPending_DryRun(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DriverName:                "mysql",
		DSN:                       "root:password@tcp(127.0.0.1:3306)/chat?charset=utf8mb4",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun: true,
	})
	if err != nil {
		t.Skip("skip dry run test if driver fails:", err)
		return
	}

	store := NewApplyStore(db)

	// In DryRun mode, RowsAffected is 0, so ok should be false and err should be nil
	ok, err := store.UpdateStatusIfPending(context.Background(), "apply-uuid-1", apply_status.AGREE)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false in dry run mode")
	}
}
