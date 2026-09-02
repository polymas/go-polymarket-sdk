// Package transport implements the SDK's internal HTTP transport behavior.
package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	sdkerrors "github.com/polymas/go-polymarket-sdk/errors"
	"github.com/polymas/go-polymarket-sdk/internal"
)

const (
	defaultMaxRetries = 2
	maxErrorBodyBytes = 4 << 10
)

type httpClient struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
}

type HTTPOption func(*httpRequestOptions)

type httpRequestOptions struct {
	headers            map[string]string
	multiParams        map[string][]string
	service            string
	idempotent         *bool
	maxRetries         *int
	ambiguousOperation string
	backoffBase        time.Duration
	backoffMax         time.Duration
}

func WithHeaders(headers map[string]string) HTTPOption {
	return func(opts *httpRequestOptions) {
		if opts.headers == nil {
			opts.headers = make(map[string]string)
		}
		for key, value := range headers {
			opts.headers[key] = value
		}
	}
}

func WithHeader(key, value string) HTTPOption { return WithHeaders(map[string]string{key: value}) }

func WithMultiParams(multiParams map[string][]string) HTTPOption {
	return func(opts *httpRequestOptions) {
		for key, values := range multiParams {
			if len(values) == 0 {
				continue
			}
			if opts.multiParams == nil {
				opts.multiParams = make(map[string][]string)
			}
			opts.multiParams[key] = append([]string(nil), values...)
		}
	}
}

func WithService(service string) HTTPOption {
	return func(opts *httpRequestOptions) { opts.service = service }
}

// WithIdempotent marks a POST-style read as safe to retry.
func WithIdempotent() HTTPOption {
	return func(opts *httpRequestOptions) {
		value := true
		opts.idempotent = &value
	}
}

func WithoutRetry() HTTPOption {
	return func(opts *httpRequestOptions) {
		value := false
		opts.idempotent = &value
		zero := 0
		opts.maxRetries = &zero
	}
}

func WithMaxRetries(maxRetries int) HTTPOption {
	return func(opts *httpRequestOptions) {
		if maxRetries < 0 {
			maxRetries = 0
		}
		opts.maxRetries = &maxRetries
	}
}

// WithAmbiguousOnTimeout marks a mutation whose timeout/cancellation must be
// reconciled before retry. Such requests are never automatically retried.
func WithAmbiguousOnTimeout(operation string) HTTPOption {
	return func(opts *httpRequestOptions) {
		opts.ambiguousOperation = operation
		value := false
		opts.idempotent = &value
		zero := 0
		opts.maxRetries = &zero
	}
}

var (
	clientCache      = make(map[string]*httpClient)
	clientCacheMutex sync.RWMutex
)

func getOrCreateClient(baseURL string) *httpClient {
	clientCacheMutex.RLock()
	client := clientCache[baseURL]
	clientCacheMutex.RUnlock()
	if client != nil {
		return client
	}
	clientCacheMutex.Lock()
	defer clientCacheMutex.Unlock()
	if client = clientCache[baseURL]; client != nil {
		return client
	}
	transport := &http.Transport{
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		Proxy:                 http.ProxyFromEnvironment,
		MaxConnsPerHost:       10,
		MaxIdleConnsPerHost:   5,
		MaxIdleConns:          50,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	client = &httpClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   internal.HTTPClientTimeout,
			Transport: transport,
		},
		headers: make(map[string]string),
	}
	clientCache[baseURL] = client
	return client
}

func Get[T any](baseURL, path string, params map[string]string, options ...HTTPOption) (*T, error) {
	return GetContext[T](context.Background(), baseURL, path, params, options...)
}

func GetContext[T any](ctx context.Context, baseURL, path string, params map[string]string, options ...HTTPOption) (*T, error) {
	body, err := do(ctx, baseURL, nethttpMethodGet, path, mergeParams(params, options), nil, "application/json", options...)
	if err != nil {
		return nil, err
	}
	return decode[T](body)
}

func Post[T any](baseURL, path string, body any, options ...HTTPOption) (*T, error) {
	return PostContext[T](context.Background(), baseURL, path, body, options...)
}

func PostContext[T any](ctx context.Context, baseURL, path string, body any, options ...HTTPOption) (*T, error) {
	encoded, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	raw, err := do(ctx, baseURL, nethttpMethodPost, path, nil, encoded, "application/json", options...)
	if err != nil {
		return nil, err
	}
	return decode[T](raw)
}

func GetRaw(baseURL, method, path string, params map[string]string, options ...HTTPOption) ([]byte, error) {
	return GetRawContext(context.Background(), baseURL, method, path, params, options...)
}

func GetRawContext(ctx context.Context, baseURL, method, path string, params map[string]string, options ...HTTPOption) ([]byte, error) {
	return do(ctx, baseURL, method, path, mergeParams(params, options), nil, "application/octet-stream", options...)
}

func PostRaw(baseURL, path string, body []byte, options ...HTTPOption) ([]byte, error) {
	return PostRawContext(context.Background(), baseURL, path, body, options...)
}

func PostRawContext(ctx context.Context, baseURL, path string, body []byte, options ...HTTPOption) ([]byte, error) {
	return do(ctx, baseURL, nethttpMethodPost, path, nil, body, "application/json", options...)
}

func DeleteRaw[T any](baseURL, path string, body []byte, options ...HTTPOption) (*T, error) {
	return DeleteRawContext[T](context.Background(), baseURL, path, body, options...)
}

func DeleteRawContext[T any](ctx context.Context, baseURL, path string, body []byte, options ...HTTPOption) (*T, error) {
	raw, err := do(ctx, baseURL, nethttpMethodDelete, path, nil, body, "application/json", options...)
	if err != nil {
		return nil, err
	}
	return decode[T](raw)
}

func GetSlice[T any](baseURL, path string, params map[string]string, options ...HTTPOption) ([]T, error) {
	return GetSliceContext[T](context.Background(), baseURL, path, params, options...)
}

func GetSliceContext[T any](ctx context.Context, baseURL, path string, params map[string]string, options ...HTTPOption) ([]T, error) {
	response, err := GetContext[[]T](ctx, baseURL, path, params, options...)
	if err != nil {
		return nil, err
	}
	if response == nil {
		return []T{}, nil
	}
	return *response, nil
}

func Delete[T any](baseURL, path string, body any, options ...HTTPOption) (*T, error) {
	return DeleteContext[T](context.Background(), baseURL, path, body, options...)
}

func DeleteContext[T any](ctx context.Context, baseURL, path string, body any, options ...HTTPOption) (*T, error) {
	encoded, err := marshalBody(body)
	if err != nil {
		return nil, err
	}
	raw, err := do(ctx, baseURL, nethttpMethodDelete, path, nil, encoded, "application/json", options...)
	if err != nil {
		return nil, err
	}
	return decode[T](raw)
}

const (
	nethttpMethodGet    = "GET"
	nethttpMethodPost   = "POST"
	nethttpMethodDelete = "DELETE"
)

func mergeParams(params map[string]string, options []HTTPOption) map[string][]string {
	opts := parseOptions(options)
	merged := make(map[string][]string, len(params)+len(opts.multiParams))
	for key, value := range params {
		merged[key] = []string{value}
	}
	for key, values := range opts.multiParams {
		merged[key] = append([]string(nil), values...)
	}
	return merged
}

func parseOptions(options []HTTPOption) *httpRequestOptions {
	opts := &httpRequestOptions{backoffBase: 100 * time.Millisecond, backoffMax: 2 * time.Second}
	for _, option := range options {
		if option != nil {
			option(opts)
		}
	}
	return opts
}

func marshalBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal body: %w", err)
	}
	return encoded, nil
}

func decode[T any](body []byte) (*T, error) {
	var result T
	if len(body) == 0 {
		return &result, nil
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

func do(ctx context.Context, baseURL, method, path string, params map[string][]string, body []byte, contentType string, options ...HTTPOption) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context must not be nil")
	}
	opts := parseOptions(options)
	client := getOrCreateClient(baseURL)
	requestURL, err := buildSafeURL(baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("failed to build safe URL: %w", err)
	}
	parsedURL, _ := url.Parse(requestURL)
	if len(params) != 0 {
		query := parsedURL.Query()
		for key, values := range params {
			for _, value := range values {
				if value != "" {
					query.Add(key, value)
				}
			}
		}
		parsedURL.RawQuery = query.Encode()
		requestURL = parsedURL.String()
	}
	service := opts.service
	if service == "" {
		service = parsedURL.Hostname()
	}
	idempotent := method == nethttpMethodGet || method == "HEAD" || method == "OPTIONS" || method == nethttpMethodDelete
	if opts.idempotent != nil {
		idempotent = *opts.idempotent
	}
	maxRetries := defaultMaxRetries
	if opts.maxRetries != nil {
		maxRetries = *opts.maxRetries
	}
	if !idempotent {
		maxRetries = 0
	}

	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		for key, value := range client.headers {
			request.Header[key] = []string{value}
		}
		for key, value := range opts.headers {
			if value != "" {
				request.Header[key] = []string{value}
			}
		}
		if len(body) != 0 && contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}

		response, requestErr := client.httpClient.Do(request)
		if requestErr != nil {
			if opts.ambiguousOperation != "" && isTimeoutOrCancellation(requestErr) {
				return nil, &sdkerrors.AmbiguousOutcomeError{Service: service, Method: method, Path: parsedURL.Path, Operation: opts.ambiguousOperation, Cause: requestErr}
			}
			if attempt < maxRetries && ctx.Err() == nil {
				if err := waitForRetry(ctx, retryDelay(nil, attempt, opts)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("%s %s request failed: %w", method, service, requestErr)
		}

		var responseReader io.Reader = response.Body
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			responseReader = io.LimitReader(response.Body, maxErrorBodyBytes+1)
		}
		responseBody, readErr := io.ReadAll(responseReader)
		response.Body.Close()
		if readErr != nil {
			if attempt < maxRetries && ctx.Err() == nil {
				if err := waitForRetry(ctx, retryDelay(response, attempt, opts)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, fmt.Errorf("read %s response: %w", service, readErr)
		}
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return responseBody, nil
		}
		apiErr := APIErrorFromResponse(service, method, parsedURL.Path, response, responseBody)
		if apiErr.Retryable && attempt < maxRetries {
			if err := waitForRetry(ctx, retryDelay(response, attempt, opts)); err != nil {
				return nil, err
			}
			continue
		}
		return nil, apiErr
	}
}

func isTimeoutOrCancellation(err error) bool {
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return stderrors.As(err, &netErr) && netErr.Timeout()
}

func retryDelay(response *http.Response, attempt int, opts *httpRequestOptions) time.Duration {
	if response != nil {
		if delay := parseRetryAfter(response.Header.Get("Retry-After"), time.Now()); delay > 0 {
			if delay > opts.backoffMax {
				return opts.backoffMax
			}
			return delay
		}
	}
	delay := opts.backoffBase << attempt
	if delay > opts.backoffMax {
		return opts.backoffMax
	}
	return delay
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}

// APIErrorFromResponse builds the SDK's stable non-2xx error without retaining
// credentials that may be present in a response payload.
func APIErrorFromResponse(service, method, path string, response *http.Response, body []byte) *sdkerrors.APIError {
	message, code := parseErrorPayload(body)
	raw := sanitizeErrorResponse(body, 500)
	message = credentialPattern.ReplaceAllString(message, `$1$2***`)
	if len(message) > 500 {
		message = message[:500] + "..."
	}
	if message == "" {
		message = raw
	}
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return &sdkerrors.APIError{
		Service:     service,
		Method:      method,
		Path:        path,
		Status:      response.StatusCode,
		RequestID:   firstHeader(response.Header, "X-Request-ID", "X-Amzn-RequestId", "CF-Ray"),
		ErrorCode:   code,
		Message:     message,
		RetryAfter:  parseRetryAfter(response.Header.Get("Retry-After"), time.Now()),
		RawResponse: raw,
		Retryable:   retryableStatus(response.StatusCode),
	}
}

// SanitizeErrorResponse removes credential-like JSON fields and limits the
// returned text. It is exported for clients (notably the relayer) that need a
// custom transport while sharing the same redaction policy.
func SanitizeErrorResponse(body []byte, maxLength int) string {
	if maxLength <= 0 {
		maxLength = 500
	}
	return sanitizeErrorResponse(body, maxLength)
}

func retryableStatus(status int) bool {
	switch status {
	case http.StatusTooEarly, http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func parseErrorPayload(body []byte) (message, code string) {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return strings.TrimSpace(string(body)), ""
	}
	for _, key := range []string{"error_code", "errorCode", "code"} {
		if value, ok := payload[key]; ok {
			code = fmt.Sprint(value)
			break
		}
	}
	for _, key := range []string{"message", "error_message", "errorMessage", "detail"} {
		if value, ok := payload[key]; ok {
			message = fmt.Sprint(value)
			break
		}
	}
	if value, ok := payload["error"]; ok {
		switch typed := value.(type) {
		case string:
			if message == "" {
				message = typed
			}
		case map[string]any:
			if message == "" && typed["message"] != nil {
				message = fmt.Sprint(typed["message"])
			}
			if code == "" && typed["code"] != nil {
				code = fmt.Sprint(typed["code"])
			}
		}
	}
	return strings.TrimSpace(message), strings.TrimSpace(code)
}

func firstHeader(header http.Header, names ...string) string {
	for _, name := range names {
		if value := header.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func buildSafeURL(baseURL, path string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	relative, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	finalURL := base.ResolveReference(relative)
	host := finalURL.Hostname()
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if finalURL.Scheme != "https" && !(finalURL.Scheme == "http" && loopback) {
		return "", fmt.Errorf("only HTTPS protocol is allowed, got: %s", finalURL.Scheme)
	}
	if finalURL.Host == "" {
		return "", fmt.Errorf("empty host in URL")
	}
	return finalURL.String(), nil
}

var credentialPattern = regexp.MustCompile(`(?i)(api[_-]?secret|api[_-]?key|passphrase|private[_-]?key|signature|authorization)(["']?\s*[:=]\s*["']?)[^"'\s,}]+`)

func sanitizeErrorResponse(body []byte, maxLength int) string {
	if len(body) == 0 {
		return ""
	}
	var value any
	if json.Unmarshal(body, &value) == nil {
		redactJSON(value)
		if encoded, err := json.Marshal(value); err == nil {
			body = encoded
		}
	}
	sanitized := credentialPattern.ReplaceAllString(string(body), `$1$2***`)
	if len(sanitized) > maxLength {
		return sanitized[:maxLength] + "..."
	}
	return sanitized
}

func redactJSON(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, item := range typed {
			lower := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if strings.Contains(lower, "secret") || strings.Contains(lower, "passphrase") || strings.Contains(lower, "privatekey") || strings.Contains(lower, "signature") || strings.Contains(lower, "authorization") || lower == "apikey" {
				typed[key] = "***"
				continue
			}
			redactJSON(item)
		}
	case []any:
		for _, item := range typed {
			redactJSON(item)
		}
	}
}
