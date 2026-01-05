package errorx

import (
	"errors"
	"fmt"
)

// CodeError 带业务错误码的自定义错误
// 实现了 error 接口，支持 %w 包装底层错误，且能被 errors.Is/errors.As 识别
type CodeError struct {
	Code  int    // 业务错误码
	Msg   string // 错误消息
	cause error  // 被包装的底层错误
}

// Error 实现 Go 标准 error 接口，使 CodeError 可作为 error 类型使用
// 当存在底层错误时，返回格式为 "消息: 底层错误"；否则仅返回消息
// 此方法在 fmt.Println(err)、log.Error(err)、fmt.Sprintf("%v", err) 等场景被隐式调用
func (e *CodeError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.cause)
	}
	return e.Msg
}

// Unwrap 实现 errors.Unwrap 接口，支持 errors.Is/errors.As 向下追溯
func (e *CodeError) Unwrap() error {
	return e.cause
}

// New 创建一个新的 CodeError
func New(code int, msg string) *CodeError {
	return &CodeError{
		Code: code,
		Msg:  msg,
	}
}

// Newf 创建一个带格式化消息的 CodeError
func Newf(code int, format string, args ...any) *CodeError {
	return &CodeError{
		Code: code,
		Msg:  fmt.Sprintf(format, args...),
	}
}

// Wrap 包装底层错误，添加业务错误码和消息
// 用法: errorx.Wrap(err, CodeNotFound, "用户不存在")
func Wrap(err error, code int, msg string) *CodeError {
	return &CodeError{
		Code:  code,
		Msg:   msg,
		cause: err,
	}
}

// Wrapf 包装底层错误，支持格式化消息
// 用法: errorx.Wrapf(err, CodeNotFound, "用户 %s 不存在", userId)
func Wrapf(err error, code int, format string, args ...any) *CodeError {
	return &CodeError{
		Code:  code,
		Msg:   fmt.Sprintf(format, args...),
		cause: err,
	}
}

// GetCode 从错误中提取业务错误码，如果不是 CodeError 则返回默认码
func GetCode(err error) int {
	var codeErr *CodeError
	if errors.As(err, &codeErr) {
		return codeErr.Code
	}
	return CodeServerBusy // 默认返回服务繁忙
}

// 业务状态码常量定义
const (
	CodeSuccess         = 1000 // 成功
	CodeInvalidParam    = 1001 // 请求参数错误
	CodeUserExist       = 1002 // 用户已存在
	CodeUserNotExist    = 1003 // 用户不存在
	CodeInvalidPassword = 1004 // 密码错误
	CodeServerBusy      = 1005 // 服务繁忙
	CodeUnauthorized    = 1006 // 未授权/认证失败
	CodeNotFound        = 1008 // 资源不存在
	CodeDBError         = 1010 // 数据库错误
	CodeCacheError      = 1011 // 缓存错误
)

// 预定义常用错误实例
// 这些实例既可直接返回，也可用于 errors.Is 比较
var (
	ErrInvalidParam = New(CodeInvalidParam, "请求参数错误")
	ErrServerBusy   = New(CodeServerBusy, "服务繁忙")
)

// IsNotFound 检查错误是否为"未找到"类型（包括 gorm.ErrRecordNotFound）
func IsNotFound(err error) bool {
	// 检查是否是 CodeError 且 Code == CodeNotFound
	var codeErr *CodeError
	if errors.As(err, &codeErr) && codeErr.Code == CodeNotFound {
		return true
	}
	// 检查底层错误消息是否包含 "record not found"
	return err != nil && err.Error() == "record not found"
}
