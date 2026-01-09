// Package repository 定义数据访问层接口和聚合结构
// 采用 Repository 模式将数据访问逻辑与业务逻辑分离
// 所有 Repository 接口在此文件定义，具体实现在各自的文件中
package repository

import (
	"errors"

	"kama_chat_server/pkg/errorx"

	"gorm.io/gorm"
)

// ==================== 错误包装辅助函数 ====================

// wrapDBError 包装数据库错误
// 根据错误类型返回不同的错误码：
//   - ErrRecordNotFound -> CodeNotFound
//   - 其他错误 -> CodeDBError
//
// err: 原始错误
// msg: 错误描述
// 返回: 包装后的错误
func wrapDBError(err error, msg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrap(err, errorx.CodeNotFound, msg)
	}
	return errorx.Wrap(err, errorx.CodeDBError, msg)
}

// wrapDBErrorf 包装数据库错误（支持格式化消息）
// 功能同 wrapDBError，但支持 fmt.Sprintf 风格的格式化
func wrapDBErrorf(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errorx.Wrapf(err, errorx.CodeNotFound, format, args...)
	}
	return errorx.Wrapf(err, errorx.CodeDBError, format, args...)
}
