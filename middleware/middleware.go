package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
	MaxRetries          int
	BackoffBase         time.Duration
	BackoffMax          time.Duration
	RetryableStatusCodes []int
}

// NewRetryMiddleware 创建重试中间件
func NewRetryMiddleware(maxRetries int, backoffBase, backoffMax time.Duration) Middleware {
	retryMW := &RetryMiddleware{
		MaxRetries:          maxRetries,
		BackoffBase:         backoffBase,
		BackoffMax:          backoffMax,
		RetryableStatusCodes: []int{500, 502, 503, 504, 408, 429},
	}
	return retryMW.Wrap
}

// Wrap 包装请求函数
func (m *RetryMiddleware) Wrap(next RequestFunc) RequestFunc {
	return func(ctx context.Context, req *http.Request) (*http.Response, error) {
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
			resp, err := next(ctx, req)
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
				// 读取并关闭响应体
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				if attempt < m.MaxRetries {
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
			internal.LogDebug("HTTP 请求: %s %s", req.Method, req.URL.String())
		}

		resp, err := next(ctx, req)

		duration := time.Since(start)

		if err != nil {
			if m.EnableRequestLog {
				internal.LogError("HTTP 请求失败: %s %s (耗时: %v): %v",
					req.Method, req.URL.String(), duration, err)
			}
			return nil, err
		}

		if m.EnableResponseLog {
			internal.LogDebug("HTTP 响应: %s %s - %d (耗时: %v)",
				req.Method, req.URL.String(), resp.StatusCode, duration)
		}

		return resp, nil
	}
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
