package database

import (
	"context"

	"gorm.io/gorm"
)

type txCtxKey struct{}

// WithTx 将事务句柄注入 Context
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txCtxKey{}, tx)
}

// GetDB 从 Context 获取事务句柄；若未开启事务则返回 defaultDB.WithContext(ctx)
func GetDB(ctx context.Context, defaultDB *gorm.DB) *gorm.DB {
	if defaultDB == nil {
		if tx, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && tx != nil {
			return tx.WithContext(ctx)
		}
		return nil
	}
	if tx, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return defaultDB.WithContext(ctx)
}

// ExecTx 统一事务执行器（支持防重入，如果在事务里直接复用）
func ExecTx(ctx context.Context, db *gorm.DB, fn func(txCtx context.Context) error) error {
	if db == nil {
		return fn(ctx)
	}
	if _, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok {
		return fn(ctx)
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}
