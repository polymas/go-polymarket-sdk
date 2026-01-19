package errors

import (
	"fmt"
	"net/http"
)

// ErrorType 错误类型
type ErrorType string

const (
	// ErrorTypeNetwork 网络错误
	ErrorTypeNetwork ErrorType = "network"
	// ErrorTypeAPI API 错误
	ErrorTypeAPI ErrorType = "api"
	// ErrorTypeValidation 验证错误
	ErrorTypeValidation ErrorType = "validation"
	// ErrorTypeAuth 认证错误
	ErrorTypeAuth ErrorType = "auth"
	// ErrorTypeRateLimit 限流错误
	ErrorTypeRateLimit ErrorType = "rate_limit"
	// ErrorTypeTimeout 超时错误
	ErrorTypeTimeout ErrorType = "timeout"
	// ErrorTypeUnknown 未知错误
	ErrorTypeUnknown ErrorType = "unknown"
)

// SDKError SDK 错误结构
type SDKError struct {
	// Type 错误类型
	Type ErrorType

	// Message 错误消息
	Message string

	// StatusCode HTTP 状态码（如果有）
	StatusCode int

	// Retryable 是否可重试
	Retryable bool

	// Cause 原始错误
	Cause error

	// Context 错误上下文信息
	Context map[string]interface{}
}

// Error 实现 error 接口
func (e *SDKError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap 返回原始错误（用于 errors.Unwrap）
func (e *SDKError) Unwrap() error {
	return e.Cause
}

// IsRetryable 判断是否可重试
func (e *SDKError) IsRetryable() bool {
	return e.Retryable
}

// NewNetworkError 创建网络错误
func NewNetworkError(message string, cause error) *SDKError {
	return &SDKError{
		Type:      ErrorTypeNetwork,
		Message:   message,
		Retryable: true,
		Cause:     cause,
	}
}

// NewAPIError 创建 API 错误
func NewAPIError(statusCode int, message string, cause error) *SDKError {
	retryable := isRetryableStatusCode(statusCode)
	return &SDKError{
		Type:       ErrorTypeAPI,
		Message:    message,
		StatusCode: statusCode,
		Retryable:  retryable,
		Cause:      cause,
	}
}

// NewValidationError 创建验证错误
func NewValidationError(message string, cause error) *SDKError {
	return &SDKError{
		Type:      ErrorTypeValidation,
		Message:   message,
		Retryable: false,
		Cause:     cause,
	}
}

// NewAuthError 创建认证错误
func NewAuthError(message string, cause error) *SDKError {
	return &SDKError{
		Type:      ErrorTypeAuth,
		Message:   message,
		Retryable: false,
		Cause:     cause,
	}
}

// NewRateLimitError 创建限流错误
func NewRateLimitError(message string, cause error) *SDKError {
	return &SDKError{
		Type:      ErrorTypeRateLimit,
		Message:   message,
		Retryable: true,
		Cause:     cause,
		Context: map[string]interface{}{
			"retry_after": true,
		},
	}
}

// NewTimeoutError 创建超时错误
func NewTimeoutError(message string, cause error) *SDKError {
	return &SDKError{
		Type:      ErrorTypeTimeout,
		Message:   message,
		Retryable: true,
		Cause:     cause,
	}
}

// WrapError 包装错误
func WrapError(err error, errorType ErrorType, message string) *SDKError {
	if sdkErr, ok := err.(*SDKError); ok {
		// 如果已经是 SDKError，更新消息和类型
		sdkErr.Message = message
		sdkErr.Type = errorType
		return sdkErr
	}

	return &SDKError{
		Type:      errorType,
		Message:   message,
		Retryable: false,
		Cause:     err,
	}
}

// isRetryableStatusCode 判断状态码是否可重试
func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		http.StatusRequestTimeout,
		http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

// IsRetryableError 判断错误是否可重试
func IsRetryableError(err error) bool {
	if sdkErr, ok := err.(*SDKError); ok {
		return sdkErr.IsRetryable()
	}
	return false
}

// GetErrorType 获取错误类型
func GetErrorType(err error) ErrorType {
	if sdkErr, ok := err.(*SDKError); ok {
		return sdkErr.Type
	}
	return ErrorTypeUnknown
}
