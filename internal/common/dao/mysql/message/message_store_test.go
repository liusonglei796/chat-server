package message

import (
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"kama_chat_server/internal/common/model"
)

func TestResolveNextCursor(t *testing.T) {
	t.Run("empty message slice returns empty cursor", func(t *testing.T) {
		cursor := resolveNextCursor([]model.Message{})
		if cursor != "" {
			t.Fatalf("expected empty cursor, got %s", cursor)
		}
	})

	t.Run("returns uuid when present", func(t *testing.T) {
		msgs := []model.Message{
			{Uuid: "M1001", Model: gorm.Model{CreatedAt: time.Now()}},
			{Uuid: "M1002", Model: gorm.Model{CreatedAt: time.Now()}},
		}
		cursor := resolveNextCursor(msgs)
		if cursor != "M1002" {
			t.Fatalf("expected M1002, got %s", cursor)
		}
	})

	t.Run("fallback to timestamp when uuid is empty", func(t *testing.T) {
		targetTime := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
		msgs := []model.Message{
			{Model: gorm.Model{CreatedAt: targetTime}},
		}
		cursor := resolveNextCursor(msgs)
		expected := "1789380000"
		if cursor != expected {
			t.Fatalf("expected timestamp %s, got %s", expected, cursor)
		}
	})
}

func TestApplyMessageCursor_SQL(t *testing.T) {
	// 使用 dryRun 模式检查 GORM 生成的 SQL 是否符合预期
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

	t.Run("empty cursor orders by uuid DESC", func(t *testing.T) {
		stmt := applyMessageCursor(db.Model(&model.Message{}).Where("session_id = ?", "S001"), "").Find(&[]model.Message{}).Statement
		sql := stmt.SQL.String()
		if stmt.SQL.String() == "" {
			t.Fatal("empty generated sql")
		}
		if !contains(sql, "ORDER BY uuid DESC") {
			t.Errorf("expected ORDER BY uuid DESC in SQL, got: %s", sql)
		}
	})

	t.Run("snowflake cursor adds uuid < ? condition", func(t *testing.T) {
		stmt := applyMessageCursor(db.Model(&model.Message{}).Where("session_id = ?", "S001"), "M1234567890").Find(&[]model.Message{}).Statement
		sql := stmt.SQL.String()
		if !contains(sql, "`uuid` < ?") && !contains(sql, "uuid < ?") {
			t.Errorf("expected uuid < ? in SQL, got: %s", sql)
		}
		if !contains(sql, "ORDER BY uuid DESC") {
			t.Errorf("expected ORDER BY uuid DESC in SQL, got: %s", sql)
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && searchSubstr(s, substr)))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
