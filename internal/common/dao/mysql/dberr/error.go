package dberr

import (
	"errors"

	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// WrapDBError 包装数据库错误
// 根据错误类型返回不同的错误码：
//
//   - ErrRecordNotFound -> CodeNotFound
//
//   - 其他错误 -> CodeDBError
//
// WrapDBError 把“数据库错误到业务码”的映射集中起来：统一处理 gorm.ErrRecordNotFound -> CodeNotFound，其它 -> CodeDBError，避免每个 DAO 重复 errors.Is 判断、保持一致的错误码和消息格式。
func WrapDBError(err error, msg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrap(err, errorx.CodeNotFound, msg)
	}
	return errorx.Wrap(err, errorx.CodeDBError, msg)
}

// WrapDBErrorf 包装数据库错误（支持格式化消息）
// 功能同 WrapDBError，但支持 fmt.Sprintf 风格的格式化
func WrapDBErrorf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrapf(err, errorx.CodeNotFound, format, args...)
	}
	return errorx.Wrapf(err, errorx.CodeDBError, format, args...)
}
