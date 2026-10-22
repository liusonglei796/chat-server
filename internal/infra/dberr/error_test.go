package dberr

import (
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
)

func TestIsDuplicateKey(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if IsDuplicateKey(nil) {
			t.Fatal("expected false for nil error")
		}
	})

	t.Run("mysql 1062 duplicate error", func(t *testing.T) {
		err := &mysql.MySQLError{
			Number:  1062,
			Message: "Duplicate entry 'test' for key 'PRIMARY'",
		}
		if !IsDuplicateKey(err) {
			t.Fatal("expected true for MySQL 1062 error")
		}
	})

	t.Run("other mysql error", func(t *testing.T) {
		err := &mysql.MySQLError{
			Number:  1050,
			Message: "Table already exists",
		}
		if IsDuplicateKey(err) {
			t.Fatal("expected false for MySQL 1050 error")
		}
	})

	t.Run("generic error", func(t *testing.T) {
		err := errors.New("something went wrong")
		if IsDuplicateKey(err) {
			t.Fatal("expected false for generic error")
		}
	})
}
