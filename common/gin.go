package common

import (
	"bytes"
	"fmt"
	"io"
	"one-api/types"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func UnmarshalBodyReusable(c *gin.Context, v any) error {
	requestBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	err = c.Request.Body.Close()
	if err != nil {
		return err
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	err = c.ShouldBind(v)
	if err != nil {
		if errs, ok := err.(validator.ValidationErrors); ok {
			// 返回第一个错误字段的名称
			return fmt.Errorf("field %s is required", errs[0].Field())
		}
		return err
	}

	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	return nil
}

func ErrorWrapper(err error, code string, statusCode int) *types.OpenAIErrorWithStatusCode {
	return StringErrorWrapper(err.Error(), code, statusCode)
}

func StringErrorWrapper(err string, code string, statusCode int) *types.OpenAIErrorWithStatusCode {
	openAIError := types.OpenAIError{
		Message: err,
		Type:    "happy-api_error",
		Code:    code,
	}
	return &types.OpenAIErrorWithStatusCode{
		OpenAIError: openAIError,
		StatusCode:  statusCode,
	}
}

func AbortWithMessage(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    "happy-api_error",
		},
	})
	c.Abort()
	LogError(c.Request.Context(), message)
}
