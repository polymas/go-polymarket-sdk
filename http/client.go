package http

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/polymas/go-polymarket-sdk/internal"
)

// httpClient HTTP客户端实现（不可导出）
type httpClient struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
}

// HTTPOption HTTP请求选项
type HTTPOption func(*httpRequestOptions)

// httpRequestOptions HTTP请求选项
type httpRequestOptions struct {
	headers     map[string]string
	multiParams map[string][]string // 同名参数（如 clob_token_ids=id1&clob_token_ids=id2）
}

// WithHeaders 设置请求头（函数选项）
func WithHeaders(headers map[string]string) HTTPOption {
	return func(opts *httpRequestOptions) {
		if opts.headers == nil {
			opts.headers = make(map[string]string)
		}
		for k, v := range headers {
			opts.headers[k] = v
		}
	}
}

// WithHeader 设置单个请求头（函数选项）
func WithHeader(key, value string) HTTPOption {
	return func(opts *httpRequestOptions) {
		if opts.headers == nil {
			opts.headers = make(map[string]string)
		}
		opts.headers[key] = value
	}
}

// WithMultiParams 设置同名参数（函数选项）
// 用于支持同名查询参数，如 clob_token_ids=id1&clob_token_ids=id2
func WithMultiParams(multiParams map[string][]string) HTTPOption {
	return func(opts *httpRequestOptions) {
		if len(multiParams) == 0 {
			return
		}
		for k, values := range multiParams {
			if len(values) > 0 {
				if opts.multiParams == nil {
					opts.multiParams = make(map[string][]string)
				}
				opts.multiParams[k] = values
			}
		}
	}
}

// getOrCreateClient 获取或创建 HTTP 客户端（使用缓存）
var clientCache = make(map[string]*httpClient)
var clientCacheMutex sync.RWMutex

func getOrCreateClient(baseURL string) *httpClient {
	clientCacheMutex.RLock()
	if client, ok := clientCache[baseURL]; ok {
		clientCacheMutex.RUnlock()
		return client
	}
	clientCacheMutex.RUnlock()

	clientCacheMutex.Lock()
	defer clientCacheMutex.Unlock()

	// 双重检查
	if client, ok := clientCache[baseURL]; ok {
		return client
	}

	// 创建安全的HTTP传输配置
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12, // 最低TLS 1.2版本
			// 不跳过证书验证，使用系统默认的证书验证
		},
	}

	client := &httpClient{
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

// Get performs a GET request (包级泛型函数)
// baseURL 为 API 基础 URL，params 为普通参数
// options 为函数选项，可用于设置请求头和同名参数等
func Get[T any](baseURL, path string, params map[string]string, options ...HTTPOption) (*T, error) {
	// 解析选项
	opts := &httpRequestOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	// 获取或创建客户端
	c := getOrCreateClient(baseURL)

	// 合并普通参数和同名参数
	var allParams map[string][]string
	if len(opts.multiParams) > 0 {
		allParams = make(map[string][]string)
		// 先添加普通参数
		for k, v := range params {
			allParams[k] = []string{v}
		}
		// 再添加同名参数（会覆盖同名普通参数）
		for k, values := range opts.multiParams {
			allParams[k] = values
		}
	} else {
		// 只有普通参数，转换为 map[string][]string
		allParams = make(map[string][]string)
		for k, v := range params {
			allParams[k] = []string{v}
		}
	}

	return request[T](c, "GET", path, allParams, nil, opts.headers)
}

// Post performs a POST request (包级泛型函数)
// baseURL 为 API 基础 URL，options 为函数选项，可用于设置请求头等
func Post[T any](baseURL, path string, body interface{}, options ...HTTPOption) (*T, error) {
	// 解析选项
	opts := &httpRequestOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	// 获取或创建客户端
	c := getOrCreateClient(baseURL)

	return request[T](c, "POST", path, nil, body, opts.headers)
}

// request performs a generic HTTP request with slice params
// 这是一个内部辅助函数，使用泛型处理响应
func request[T any](c *httpClient, method, path string, params map[string][]string, body interface{}, requestHeaders map[string]string) (*T, error) {
	req, err := buildRequestWithSliceParams(c, method, path, params, body, "application/json", requestHeaders)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sanitizedBody := sanitizeErrorResponse(responseBodyBytes, 500)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, sanitizedBody)
	}

	var result T
	if len(responseBodyBytes) == 0 {
		return &result, nil
	}

	if err := json.Unmarshal(responseBodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetRaw performs a request and returns raw bytes
// baseURL 为 API 基础 URL，params 为普通参数
// options 为函数选项，可用于设置请求头和同名参数等
func GetRaw(baseURL, method, path string, params map[string]string, options ...HTTPOption) ([]byte, error) {
	// 解析选项
	opts := &httpRequestOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	// 获取或创建客户端
	c := getOrCreateClient(baseURL)

	// 合并普通参数和同名参数
	var allParams map[string][]string
	if len(opts.multiParams) > 0 {
		allParams = make(map[string][]string)
		// 先添加普通参数
		for k, v := range params {
			allParams[k] = []string{v}
		}
		// 再添加同名参数（会覆盖同名普通参数）
		for k, values := range opts.multiParams {
			allParams[k] = values
		}
	} else {
		// 只有普通参数，转换为 map[string][]string
		allParams = make(map[string][]string)
		for k, v := range params {
			allParams[k] = []string{v}
		}
	}

	req, err := buildRequestWithSliceParams(c, method, path, allParams, nil, "application/octet-stream", opts.headers)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		sanitizedBody := sanitizeErrorResponse(bodyBytes, 500)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, sanitizedBody)
	}

	return io.ReadAll(resp.Body)
}

// PostRaw performs a POST request with raw body bytes and returns raw bytes
// baseURL 为 API 基础 URL，用于发送预格式化的 JSON（如带空格的 JSON，匹配 Python 的 json.dumps 格式）
// options 为函数选项，可用于设置请求头等
func PostRaw(baseURL, path string, bodyBytes []byte, options ...HTTPOption) ([]byte, error) {
	// 解析选项
	opts := &httpRequestOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	// 获取或创建客户端
	c := getOrCreateClient(baseURL)

	// 使用安全的URL构建方法
	requestURL, err := buildSafeURL(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("failed to build safe URL: %w", err)
	}

	req, err := http.NewRequest("POST", requestURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers (preserve exact case)
	// IMPORTANT: Set Content-Type first, then other headers (matching Python httpx behavior)
	req.Header.Set("Content-Type", "application/json")

	// 先设置客户端默认 headers
	for k, v := range c.headers {
		if len(v) > 0 {
			req.Header[k] = []string{v}
		}
	}

	// 再设置请求特定的 headers（会覆盖默认 headers）
	for k, v := range opts.headers {
		if len(v) > 0 {
			req.Header[k] = []string{v}
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sanitizedBody := sanitizeErrorResponse(responseBody, 500)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, sanitizedBody)
	}

	return responseBody, nil
}

// DeleteRaw performs a DELETE request with raw body bytes and returns parsed response
// baseURL 为 API 基础 URL，用于发送预格式化的 JSON（如带空格的 JSON，匹配 Python 的 json.dumps 格式）
// options 为函数选项，可用于设置请求头等
func DeleteRaw[T any](baseURL, path string, bodyBytes []byte, options ...HTTPOption) (*T, error) {
	// 解析选项
	opts := &httpRequestOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	// 获取或创建客户端
	c := getOrCreateClient(baseURL)

	// 使用安全的URL构建方法
	requestURL, err := buildSafeURL(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("failed to build safe URL: %w", err)
	}

	req, err := http.NewRequest("DELETE", requestURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers (preserve exact case)
	// IMPORTANT: Set Content-Type first, then other headers (matching Python httpx behavior)
	req.Header.Set("Content-Type", "application/json")

	// 先设置客户端默认 headers
	for k, v := range c.headers {
		if len(v) > 0 {
			req.Header[k] = []string{v}
		}
	}

	// 再设置请求特定的 headers（会覆盖默认 headers）
	for k, v := range opts.headers {
		if len(v) > 0 {
			req.Header[k] = []string{v}
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(rawBytes))
	}

	var result T
	if len(rawBytes) == 0 {
		return &result, nil
	}

	if err := json.Unmarshal(rawBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// buildSafeURL 安全地构建URL，防止SSRF攻击
func buildSafeURL(baseURL, path string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	rel, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	finalURL := base.ResolveReference(rel)

	// 安全验证：只允许HTTPS协议
	if finalURL.Scheme != "https" {
		return "", fmt.Errorf("only HTTPS protocol is allowed, got: %s", finalURL.Scheme)
	}

	// 验证主机名不为空
	if finalURL.Host == "" {
		return "", fmt.Errorf("empty host in URL")
	}

	return finalURL.String(), nil
}

// buildRequestWithSliceParams builds an HTTP request with slice params (支持同名参数)
func buildRequestWithSliceParams(c *httpClient, method, path string, params map[string][]string, body interface{}, contentType string, requestHeaders map[string]string) (*http.Request, error) {
	// 使用安全的URL构建方法
	requestURL, err := buildSafeURL(c.baseURL, path)
	if err != nil {
		return nil, fmt.Errorf("failed to build safe URL: %w", err)
	}

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers (preserve exact case)
	// 先设置客户端默认 headers
	for k, v := range c.headers {
		req.Header[k] = []string{v}
	}

	// 再设置请求特定的 headers（会覆盖默认 headers）
	for k, v := range requestHeaders {
		if len(v) > 0 {
			req.Header[k] = []string{v}
		}
	}

	if body != nil && contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if len(params) > 0 {
		q := req.URL.Query()
		for k, values := range params {
			for _, v := range values {
				if v != "" {
					q.Add(k, v)
				}
			}
		}
		req.URL.RawQuery = q.Encode()
	}

	return req, nil
}

// sanitizeErrorResponse 清理错误响应中的敏感信息
// maxLen: 最大返回长度（超过则截断）
func sanitizeErrorResponse(body []byte, maxLen int) string {
	if len(body) == 0 {
		return ""
	}

	bodyStr := string(body)

	// 移除可能的敏感字段
	sensitiveFields := []string{
		`"secret"`,
		`"apiKey"`,
		`"api_key"`,
		`"passphrase"`,
		`"privateKey"`,
		`"private_key"`,
		`"token"`,
		`"access_token"`,
		`"refresh_token"`,
	}

	for _, field := range sensitiveFields {
		// 替换敏感字段的值
		// 匹配: "field": "value"
		re := strings.NewReplacer(field+`": "`, field+`": "***"`)
		bodyStr = re.Replace(bodyStr)
		// 也处理不带引号的值: "field": value
		re2 := strings.NewReplacer(field+`": `, field+`": ***`)
		bodyStr = re2.Replace(bodyStr)
	}

	// 限制长度
	if len(bodyStr) > maxLen {
		return bodyStr[:maxLen] + "..."
	}

	return bodyStr
}

// GetSlice performs a GET request and returns a slice, handling nil response
func GetSlice[T any](baseURL, path string, params map[string]string, options ...HTTPOption) ([]T, error) {
	resp, err := Get[[]T](baseURL, path, params, options...)
	if err != nil {
		return nil, err
	}
	if resp != nil {
		return *resp, nil
	}
	return []T{}, nil
}

// Delete performs a DELETE request
func Delete[T any](baseURL, path string, body interface{}, options ...HTTPOption) (*T, error) {
	// 解析选项
	opts := &httpRequestOptions{}
	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	// 获取或创建客户端
	c := getOrCreateClient(baseURL)

	req, err := buildRequestWithSliceParams(c, "DELETE", path, nil, body, "application/json", opts.headers)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	rawBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(rawBytes))
	}

	var result T
	if len(rawBytes) == 0 {
		return &result, nil
	}

	if err := json.Unmarshal(rawBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
