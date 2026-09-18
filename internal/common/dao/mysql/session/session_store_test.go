package session

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"kama_chat_server/internal/common/model"
)

func TestSessionCursorEncodeDecode(t *testing.T) {
	t.Run("pinned session encode and parse", func(t *testing.T) {
		targetTime := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
		s := model.Session{
			IsPinned:      true,
			LastMessageAt: sql.NullTime{Time: targetTime, Valid: true},
		}
		cursor := encodeSessionCursor(s)
		if cursor != "1_1789560000" {
			t.Fatalf("expected cursor 1_1789560000, got %s", cursor)
		}

		isPinned, ts, ok := parseSessionCursor(cursor)
		if !ok || !isPinned || ts != 1789560000 {
			t.Fatalf("parse failed: isPinned=%v, ts=%d, ok=%v", isPinned, ts, ok)
		}
	})

	t.Run("unpinned session encode and parse", func(t *testing.T) {
		targetTime := time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)
		s := model.Session{
			IsPinned:      false,
			LastMessageAt: sql.NullTime{Time: targetTime, Valid: true},
		}
		cursor := encodeSessionCursor(s)
		if cursor != "0_1789552800" {
			t.Fatalf("expected cursor 0_1789552800, got %s", cursor)
		}

		isPinned, ts, ok := parseSessionCursor(cursor)
		if !ok || isPinned || ts != 1789552800 {
			t.Fatalf("parse failed: isPinned=%v, ts=%d, ok=%v", isPinned, ts, ok)
		}
	})

	t.Run("colon separator format parse", func(t *testing.T) {
		isPinned, ts, ok := parseSessionCursor("1:1789560000")
		if !ok || !isPinned || ts != 1789560000 {
			t.Fatalf("parse colon cursor failed: isPinned=%v, ts=%d, ok=%v", isPinned, ts, ok)
		}
	})

	t.Run("legacy timestamp cursor fallback parse", func(t *testing.T) {
		isPinned, ts, ok := parseSessionCursor("1789552800")
		if !ok || isPinned || ts != 1789552800 {
			t.Fatalf("legacy parse failed: isPinned=%v, ts=%d, ok=%v", isPinned, ts, ok)
		}
	})

	t.Run("empty or invalid cursor parse", func(t *testing.T) {
		_, _, ok := parseSessionCursor("")
		if ok {
			t.Fatal("expected ok=false on empty cursor")
		}
		_, _, ok2 := parseSessionCursor("invalid_string_here")
		if ok2 {
			t.Fatal("expected ok=false on invalid cursor")
		}
	})
}

func TestFindBySendIdAndTypeCursor_DryRunSQL(t *testing.T) {
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

	store := NewSessionStore(db)

	t.Run("pinned cursor generates or unpinned condition", func(t *testing.T) {
		res, err := store.FindBySendIdAndTypeCursor(context.Background(), "U1001", "U", "1_1789560000", 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil result in dry run")
		}
	})

	t.Run("unpinned cursor generates unpinned condition", func(t *testing.T) {
		res, err := store.FindBySendIdAndTypeCursor(context.Background(), "U1001", "U", "0_1789552800", 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil result in dry run")
		}
	})
}
