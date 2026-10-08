package group

import (
	"context"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestGroupStore_UpdateWithVersion_DryRun(t *testing.T) {
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

	store := NewGroupStore(db)

	updates := map[string]interface{}{
		"name":   "Test Group",
		"notice": "Welcome",
	}

	// In DryRun mode, db operations do not actually execute against a server,
	// but generate the SQL statement. RowsAffected is 0 in DryRun, so UpdateWithVersion returns errorx.CodeConflict.
	err = store.UpdateWithVersion(context.Background(), "G1001", 1, updates)
	if err == nil {
		t.Fatal("expected conflict error due to RowsAffected==0 in dry run, got nil")
	}
}
