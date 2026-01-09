// Package internal 定义数据访问层内部共享的辅助函数
// 提供数据库错误包装等工具函数，供各个repository子包使用
package internal

import (
	"errors"

	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// ==================== 错误包装辅助函数 ====================

// WrapDBError 包装数据库错误
// 根据错误类型返回不同的错误码：
//   - ErrRecordNotFound -> CodeNotFound
//   - 其他错误 -> CodeDBError
//
// err: 原始错误
// msg: 错误描述
// 返回: 包装后的错误
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
