// Package handler 提供 HTTP 请求处理器
// 本文件定义统一响应格式和错误处理方法
package handler

import (
	"errors"
	"net/http"

	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// ResponseData 统一响应结构体
// 用于 Swagger 文档生成和 API 响应规范
type ResponseData struct {
	Code int `json:"code"`           // 业务响应状态码，参见 pkg/errorx/errorx.go
	Msg  any `json:"msg"`            // 提示信息，成功时为 "success"，失败时为错误描述
	Data any `json:"data,omitempty"` // 响应数据，可选
}

// HandleSuccess 返回成功响应
// 所有成功的 API 响应都应使用此方法
// c: Gin 上下文
// data: 要返回的数据，可以是任意类型
func HandleSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code": errorx.CodeSuccess, // 1000 表示成功
		"msg":  "success",
		"data": data,
	})
}

// HandleError 通用错误处理方法
// 自动识别并处理两种类型的错误：
//  1. errorx.CodeError - 业务错误：返回携带的错误码和消息
//  2. 其他错误 - 系统错误：记录日志并返回"服务繁忙"
//
// 使用示例:
//
//	if err := service.DoSomething(); err != nil {
//	    HandleError(c, err)
//	    return
//	}
func HandleError(c *gin.Context, err error) {
	// 1. 尝试断言为业务错误类型 *errorx.CodeError
	var codeErr *errorx.CodeError
	if errors.As(err, &codeErr) {
		// 业务错误：直接返回错误码和消息
		c.JSON(http.StatusOK, gin.H{
			"code": codeErr.Code,
			"msg":  codeErr.Msg,
			"data": nil,
		})
		return
	}

	// 2. 系统错误或未知错误：记录详细日志，返回通用错误信息
	// 避免将内部错误信息暴露给客户端
	zap.L().Error("system error",
		zap.String("path", c.Request.URL.Path), // 请求路径
		zap.String("method", c.Request.Method), // 请求方法
		zap.Error(err),                         // 原始错误
	)
	c.JSON(http.StatusOK, gin.H{
		"code": errorx.ErrServerBusy.Code, // 返回通用错误码
		"msg":  errorx.ErrServerBusy.Msg,  // "服务繁忙，请稍后重试"
		"data": nil,
	})
}

// HandleParamError 处理参数绑定错误
// 专门用于处理 c.ShouldBindJSON() 等绑定方法返回的错误
// 支持 validator 验证错误的中文翻译
//
// 使用示例:
//
//	var req request.LoginRequest
//	if err := c.ShouldBindJSON(&req); err != nil {
//	    HandleParamError(c, err)
//	    return
//	}
func HandleParamError(c *gin.Context, err error) {
	// 尝试断言为 validator.ValidationErrors 类型
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		// 是 validator 验证错误：进行中文翻译并去除结构体名前缀
		// 例如：将 "LoginRequest.Telephone必须填写" 转为 "Telephone必须填写"
		translatedErrs := RemoveTopStruct(validationErrs.Translate(Trans))
		c.JSON(http.StatusOK, gin.H{
			"code": errorx.ErrInvalidParam.Code,
			"msg":  translatedErrs, // 返回翻译后的错误信息 map
			"data": nil,
		})
		return
	}

	// 非 validator 错误（如 JSON 格式错误、类型不匹配等）
	zap.L().Error("param bind error", zap.Error(err))
	c.JSON(http.StatusOK, gin.H{
		"code": errorx.ErrInvalidParam.Code,
		"msg":  errorx.ErrInvalidParam.Msg, // "参数错误"
		"data": nil,
	})
}
