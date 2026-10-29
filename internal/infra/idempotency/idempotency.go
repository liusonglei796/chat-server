package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"kama_chat_server/internal/infra/database"
	"kama_chat_server/pkg/errorx"
)

var (
	ErrProcessing = errors.New("idempotency request is processing")
	ErrConflict   = errors.New("idempotency key belongs to another request")
)

type Status string

const (
	StatusProcessing Status = "PROCESSING"
	StatusSuccess    Status = "SUCCESS"
	StatusFail       Status = "FAIL"
)

// Record MySQL 幂等持久化记录表
type Record struct {
	ID             uint      `gorm:"primaryKey"`
	UserID         string    `gorm:"size:64;not null;uniqueIndex:uk_idempotency"`
	Operation      string    `gorm:"size:128;not null;uniqueIndex:uk_idempotency"`
	IdempotencyKey string    `gorm:"size:128;not null;uniqueIndex:uk_idempotency"`
	Status         Status    `gorm:"size:20;not null"`
	Retryable      bool      `gorm:"not null;default:false"`
	ErrorCode      int       `gorm:"default:0"`
	Response       []byte    `gorm:"type:json"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (Record) TableName() string { return "idempotency_record" }

type Store interface {
	Begin(ctx context.Context, userID, operation, key string) (*Record, error)
	Complete(ctx context.Context, recordID uint, response []byte) error
	Fail(ctx context.Context, recordID uint, err error) error
}

type GormStore struct {
	db *gorm.DB
}

func NewGormStore(db *gorm.DB) Store {
	return &GormStore{db: db}
}

func Migrate(db *gorm.DB) error {
	if db == nil {
		return errors.New("idempotency database is nil")
	}
	return db.AutoMigrate(&Record{})
}

func (s *GormStore) Begin(ctx context.Context, userID, operation, key string) (*Record, error) {
	db := database.GetDB(ctx, s.db)
	if db == nil {
		return nil, errors.New("idempotency database is nil")
	}

	var record Record
	err := db.Where("user_id = ? AND operation = ? AND idempotency_key = ?", userID, operation, key).
		First(&record).Error
	if err == nil {
		switch record.Status {
		case StatusProcessing:
			return nil, ErrProcessing
		case StatusSuccess:
			return &record, nil
		case StatusFail:
			if !record.Retryable {
				return &record, nil
			}
			now := time.Now()
			res := db.Model(&Record{}).
				Where("id = ? AND status = ? AND retryable = ?", record.ID, StatusFail, true).
				Updates(map[string]interface{}{
					"status":     StatusProcessing,
					"updated_at": now,
				})
			if res.Error != nil {
				return nil, res.Error
			}
			if res.RowsAffected == 0 {
				return nil, ErrProcessing
			}
			record.Status = StatusProcessing
			record.UpdatedAt = now
			return &record, nil
		}
		return nil, ErrConflict
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	record = Record{
		UserID:         userID,
		Operation:      operation,
		IdempotencyKey: key,
		Status:         StatusProcessing,
	}
	if err := db.Create(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return s.Begin(ctx, userID, operation, key)
		}
		return nil, err
	}
	return &record, nil
}

func (s *GormStore) Complete(ctx context.Context, recordID uint, response []byte) error {
	db := database.GetDB(ctx, s.db)
	if db == nil {
		return errors.New("idempotency database is nil")
	}
	if !json.Valid(response) {
		return errors.New("idempotency response is not valid JSON")
	}
	return db.Model(&Record{}).Where("id = ? AND status = ?", recordID, StatusProcessing).
		Updates(map[string]interface{}{
			"status":   StatusSuccess,
			"response": response,
		}).Error
}

func (s *GormStore) Fail(ctx context.Context, recordID uint, err error) error {
	db := database.GetDB(ctx, s.db)
	if db == nil {
		return errors.New("idempotency database is nil")
	}
	retryable, code, msg := isRetryable(err)
	resp, _ := json.Marshal(map[string]interface{}{
		"code":    code,
		"message": msg,
	})
	return db.Model(&Record{}).Where("id = ? AND status = ?", recordID, StatusProcessing).
		Updates(map[string]interface{}{
			"status":     StatusFail,
			"retryable":  retryable,
			"error_code": code,
			"response":   resp,
			"updated_at": time.Now(),
		}).Error
}

func isRetryable(err error) (bool, int, string) {
	if err == nil {
		return true, 0, ""
	}
	var codeErr *errorx.CodeError
	if errors.As(err, &codeErr) {
		code := codeErr.Code
		msg := codeErr.Msg
		switch code {
		case errorx.CodeInvalidParam,
			errorx.CodeUserExist,
			errorx.CodeUserNotExist,
			errorx.CodeInvalidPassword,
			errorx.CodeUnauthorized,
			errorx.CodeForbidden,
			errorx.CodeNotFound:
			return false, code, msg
		default:
			// 5xx, CodeServerBusy, CodeTimeout, CodeDBError, CodeConflict, CodeTooManyRequests
			return true, code, msg
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true, errorx.CodeTimeout, "请求超时"
	}
	return false, errorx.CodeServerBusy, err.Error()
}

// ExecuteJSON 封装通用的幂等表操作逻辑
func ExecuteJSON[T any](
	ctx context.Context,
	store Store,
	userID, operation, key string,
	fn func(execCtx context.Context) (*T, error),
) (*T, bool, error) {
	if store == nil || key == "" {
		res, err := fn(ctx)
		return res, false, err
	}

	record, err := store.Begin(ctx, userID, operation, key)
	if err != nil {
		if errors.Is(err, ErrProcessing) {
			return nil, false, errorx.New(errorx.CodeTooManyRequests, "正在处理中，请勿重复提交")
		}
		return nil, false, err
	}

	// 1. 若已成功，直接幂等回放结果
	if record.Status == StatusSuccess {
		var replay T
		if len(record.Response) > 0 {
			if err := json.Unmarshal(record.Response, &replay); err == nil {
				return &replay, true, nil
			}
		}
		return nil, true, nil
	}

	// 2. 若不可重试的失败，回放历史错误
	if record.Status == StatusFail && !record.Retryable {
		return nil, false, errorx.New(record.ErrorCode, "前序请求已终态失败，不可重试")
	}

	// 3. 执行业务闭包
	res, err := fn(ctx)
	if err != nil {
		_ = store.Fail(ctx, record.ID, err)
		return nil, false, err
	}

	// 4. 持久化成功响应
	respBytes, mErr := json.Marshal(res)
	if mErr == nil {
		_ = store.Complete(ctx, record.ID, respBytes)
	}

	return res, false, nil
}

func ContextWithKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, contextKey{}, key)
}

func KeyFromContext(ctx context.Context) string {
	key, _ := ctx.Value(contextKey{}).(string)
	return key
}

type contextKey struct{}
