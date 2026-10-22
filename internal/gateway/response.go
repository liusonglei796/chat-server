package gateway

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"

	"kama_chat_server/pkg/errorx"
)

// ResponseData 统一响应结构体
type ResponseData struct {
	Code int `json:"code"`
	Msg  any `json:"msg"`
	Data any `json:"data,omitempty"`
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
func HandleError(c *gin.Context, err error) {
	var codeErr *errorx.CodeError
	if errors.As(err, &codeErr) {
		if codeErr.Code == errorx.CodeServerBusy || codeErr.Code == errorx.CodeDBError || codeErr.Code == errorx.CodeCacheError {
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
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code": codeErr.Code,
			"msg":  codeErr.Msg,
			"data": nil,
		})
		return
	}

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

// HandleParamError 处理参数绑定错误
func HandleParamError(c *gin.Context, err error) {
	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		translatedErrs := RemoveTopStruct(validationErrs.Translate(Trans))
		c.JSON(http.StatusOK, gin.H{
			"code": errorx.ErrInvalidParam.Code,
			"msg":  translatedErrs,
			"data": nil,
		})
		return
	}
	zap.L().Error("param bind error", zap.Error(err))
	c.JSON(http.StatusOK, gin.H{
		"code": errorx.ErrInvalidParam.Code,
		"msg":  errorx.ErrInvalidParam.Msg,
		"data": nil,
	})
}
