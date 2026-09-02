package errors

import (
	"errors"
	"fmt"
	"time"
)

// APIError is the stable error returned for non-2xx API responses.
// RawResponse is length-limited and has credential-like fields redacted.
type APIError struct {
	Service     string
	Method      string
	Path        string
	Status      int
	RequestID   string
	ErrorCode   string
	Message     string
	RetryAfter  time.Duration
	RawResponse string
	Retryable   bool
}

func (e *APIError) Error() string {
	location := e.Path
	if e.Service != "" {
		location = e.Service + " " + location
	}
	code := ""
	if e.ErrorCode != "" {
		code = " code=" + e.ErrorCode
	}
	requestID := ""
	if e.RequestID != "" {
		requestID = " request_id=" + e.RequestID
	}
	return fmt.Sprintf("%s %s: HTTP %d%s%s: %s", e.Method, location, e.Status, code, requestID, e.Message)
}

// IsRetryable reports whether the server status normally permits a retry.
// Mutating callers must still reconcile before retrying an ambiguous request.
func (e *APIError) IsRetryable() bool { return e.Retryable }

// AmbiguousOutcomeError means a mutating request timed out or was canceled
// after submission may have begun. The caller must reconcile state before any
// retry; the SDK intentionally does not resend it automatically.
type AmbiguousOutcomeError struct {
	Service   string
	Method    string
	Path      string
	Operation string
	Cause     error
}

func (e *AmbiguousOutcomeError) Error() string {
	operation := e.Operation
	if operation == "" {
		operation = e.Method + " " + e.Path
	}
	return fmt.Sprintf("ambiguous outcome for %s: reconcile before retry: %v", operation, e.Cause)
}

func (e *AmbiguousOutcomeError) Unwrap() error { return e.Cause }

// IsAmbiguousOutcome identifies requests that must be reconciled before retry.
func IsAmbiguousOutcome(err error) bool {
	var ambiguous *AmbiguousOutcomeError
	return errors.As(err, &ambiguous)
}

// RelayFailedError 表示 Gasless Relayer 返回 STATE_FAILED 时的错误，
// 上层可通过 errors.As(err, &relayErr) 获取 State、TransactionID 等用于日志或重试。
type RelayFailedError struct {
	State         string // 如 "STATE_FAILED"
	TransactionID string // Relayer 侧事务 ID，可用于排查
	Message       string // 可读错误信息（若 Relayer 未返回则为 "交易提交失败"）
}

func (e *RelayFailedError) Error() string {
	if e.TransactionID != "" {
		return fmt.Sprintf("交易提交失败 (state: %s, transactionID: %s): %s", e.State, e.TransactionID, e.Message)
	}
	return fmt.Sprintf("交易提交失败 (state: %s): %s", e.State, e.Message)
}

// RelayHTTPError 表示 Gasless Relayer 业务端点（/submit 等）返回非 200 的 HTTP 响应。
// 上层可通过 errors.As(err, &httpErr) 拿到状态码做针对性处理（401 重新鉴权、429 限流等）。
type RelayHTTPError struct {
	Status int    // HTTP 状态码
	Body   string // 已脱敏且可能被截断的响应体
	API    *APIError
}

func (e *RelayHTTPError) Error() string {
	if e.API != nil {
		return e.API.Error()
	}
	return fmt.Sprintf("relay returned error: HTTP %d: %s", e.Status, e.Body)
}

func (e *RelayHTTPError) Unwrap() error { return e.API }
