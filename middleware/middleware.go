package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/polymas/go-polymarket-sdk/errors"
	"github.com/polymas/go-polymarket-sdk/internal"
)

// RequestFunc 请求函数类型
type RequestFunc func(ctx context.Context, req *http.Request) (*http.Response, error)

// Middleware 中间件类型
type Middleware func(RequestFunc) RequestFunc

// Chain 链式组合多个中间件
func Chain(middlewares ...Middleware) Middleware {
	return func(next RequestFunc) RequestFunc {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// RetryMiddleware 重试中间件
type RetryMiddleware struct {
	MaxRetries           int
	BackoffBase          time.Duration
	BackoffMax           time.Duration
	RetryableStatusCodes []int
}

// NewRetryMiddleware 创建重试中间件
func NewRetryMiddleware(maxRetries int, backoffBase, backoffMax time.Duration) Middleware {
	retryMW := &RetryMiddleware{
		MaxRetries:           maxRetries,
		BackoffBase:          backoffBase,
		BackoffMax:           backoffMax,
		RetryableStatusCodes: []int{425, 500, 502, 503, 504, 408, 429},
	}
	return retryMW.Wrap
}

// Wrap 包装请求函数
func (m *RetryMiddleware) Wrap(next RequestFunc) RequestFunc {
	return func(ctx context.Context, req *http.Request) (*http.Response, error) {
		if !isIdempotentMethod(req.Method) || (req.Body != nil && req.GetBody == nil) {
			return next(ctx, req.WithContext(ctx))
		}
		var lastErr error
		var lastResp *http.Response

		for attempt := 0; attempt <= m.MaxRetries; attempt++ {
			if attempt > 0 {
				// 计算退避时间
				backoff := m.BackoffBase * time.Duration(attempt)
				if backoff > m.BackoffMax {
					backoff = m.BackoffMax
				}

				internal.LogDebug("重试请求 (尝试 %d/%d)，等待 %v", attempt, m.MaxRetries, backoff)

				// 等待退避时间
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(backoff):
				}
			}

			// 执行请求
			attemptReq := req.Clone(ctx)
			if req.Body != nil && req.GetBody != nil {
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return nil, bodyErr
				}
				attemptReq.Body = body
			}
			resp, err := next(ctx, attemptReq)
			if err != nil {
				lastErr = err
				// 检查是否可重试
				if !errors.IsRetryableError(err) {
					return nil, err
				}
				continue
			}

			// 检查状态码是否可重试
			if m.isRetryableStatusCode(resp.StatusCode) {
				lastResp = resp
				if attempt < m.MaxRetries {
					// Only consume a response when another attempt will follow.
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
					internal.LogWarn("收到可重试状态码 %d，将重试", resp.StatusCode)
					continue
				}
			}

			return resp, nil
		}

		if lastResp != nil {
			return lastResp, nil
		}

		if lastErr != nil {
			return nil, errors.WrapError(lastErr, errors.ErrorTypeNetwork,
				fmt.Sprintf("请求失败，已重试 %d 次", m.MaxRetries))
		}

		return nil, errors.NewNetworkError("请求失败，已达到最大重试次数", nil)
	}
}

func isIdempotentMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodDelete:
		return true
	default:
		return false
	}
}

// isRetryableStatusCode 判断状态码是否可重试
func (m *RetryMiddleware) isRetryableStatusCode(statusCode int) bool {
	for _, code := range m.RetryableStatusCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// LoggingMiddleware 日志中间件
type LoggingMiddleware struct {
	EnableRequestLog  bool
	EnableResponseLog bool
}

// NewLoggingMiddleware 创建日志中间件
func NewLoggingMiddleware(enableRequestLog, enableResponseLog bool) Middleware {
	logMW := &LoggingMiddleware{
		EnableRequestLog:  enableRequestLog,
		EnableResponseLog: enableResponseLog,
	}
	return logMW.Wrap
}

// Wrap 包装请求函数
func (m *LoggingMiddleware) Wrap(next RequestFunc) RequestFunc {
	return func(ctx context.Context, req *http.Request) (*http.Response, error) {
		start := time.Now()

		if m.EnableRequestLog {
			internal.LogDebug("HTTP 请求: %s %s", req.Method, redactedURL(req.URL))
		}

		resp, err := next(ctx, req)

		duration := time.Since(start)

		if err != nil {
			if m.EnableRequestLog {
				internal.LogError("HTTP 请求失败: %s %s (耗时: %v): %v",
					req.Method, redactedURL(req.URL), duration, err)
			}
			return nil, err
		}

		if m.EnableResponseLog {
			internal.LogDebug("HTTP 响应: %s %s - %d (耗时: %v)",
				req.Method, redactedURL(req.URL), resp.StatusCode, duration)
		}

		return resp, nil
	}
}

func redactedURL(input *url.URL) string {
	if input == nil {
		return ""
	}
	copyURL := *input
	query := copyURL.Query()
	for key := range query {
		normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
		if strings.Contains(normalized, "secret") || strings.Contains(normalized, "passphrase") || strings.Contains(normalized, "privatekey") || strings.Contains(normalized, "signature") || strings.Contains(normalized, "authorization") || normalized == "apikey" {
			query.Set(key, "***")
		}
	}
	copyURL.RawQuery = query.Encode()
	return copyURL.String()
}

// TimeoutMiddleware 超时中间件
type TimeoutMiddleware struct {
	Timeout time.Duration
}

// NewTimeoutMiddleware 创建超时中间件
func NewTimeoutMiddleware(timeout time.Duration) Middleware {
	timeoutMW := &TimeoutMiddleware{
		Timeout: timeout,
	}
	return timeoutMW.Wrap
}

// Wrap 包装请求函数
func (m *TimeoutMiddleware) Wrap(next RequestFunc) RequestFunc {
	return func(ctx context.Context, req *http.Request) (*http.Response, error) {
		ctx, cancel := context.WithTimeout(ctx, m.Timeout)
		defer cancel()

		return next(ctx, req)
	}
}
