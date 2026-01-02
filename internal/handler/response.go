package handler

import (
	"errors"
	"net/http"

	"kama_chat_server/pkg/errorx"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// ResponseData 统一响应结构体 (用于 Swagger 文档生成)
type ResponseData struct {
	Code int `json:"code"`           // 业务响应状态码
	Msg  any `json:"msg"`            // 提示信息
	Data any `json:"data,omitempty"` // 数据
}

// HandleSuccess 返回成功响应
func HandleSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code": errorx.CodeSuccess,
		"msg":  "success",
		"data": data,
	})
}

// HandleError 通用错误处理方法
// 自动识别 errorx.CodeError 类型的业务错误，或者将系统错误转换为 CodeServerBusy
// 使用示例：
//
//	if err := logic.DoSomething(); err != nil {
//	    HandleError(c, err)
//	    return
//	}
func HandleError(c *gin.Context, err error) {
	// 1. 尝试断言为 *errorx.CodeError 类型
	var codeErr *errorx.CodeError
	if errors.As(err, &codeErr) {
		// 业务错误：直接返回携带的错误码和消息
		c.JSON(http.StatusOK, gin.H{
			"code": codeErr.Code,
			"msg":  codeErr.Msg,
			"data": nil,
		})
		return
	}

	// 2. 系统错误或未知错误：记录日志并返回服务繁忙
	zap.L().Error("system error",
		zap.String("path", c.Request.URL.Path),
		zap.String("method", c.Request.Method),
		zap.Error(err),
	)
	c.JSON(http.StatusOK, gin.H{
		"code": errorx.ErrServerBusy.Code,
		"msg":  errorx.ErrServerBusy.Msg,
		"data": nil,
	})
}

// HandleParamError 处理参数绑定错误（带 validator 翻译支持）
// 自动识别 validator.ValidationErrors 类型并进行翻译
func HandleParamError(c *gin.Context, err error) {
	// 尝试断言为 validator.ValidationErrors 类型
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		// validator.ValidationErrors类型错误则进行翻译
		// 翻译后去除结构体名前缀，提升用户体验
		translatedErrs := RemoveTopStruct(validationErrs.Translate(Trans))
		c.JSON(http.StatusOK, gin.H{
			"code": errorx.ErrInvalidParam.Code,
			"msg":  translatedErrs,
			"data": nil,
		})
		return
	}

	// 非 validator 错误（如 JSON 格式错误）
	zap.L().Error("param bind error", zap.Error(err))
	c.JSON(http.StatusOK, gin.H{
		"code": errorx.ErrInvalidParam.Code,
		"msg":  errorx.ErrInvalidParam.Msg,
		"data": nil,
	})
}
