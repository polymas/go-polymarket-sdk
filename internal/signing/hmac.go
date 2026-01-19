package signing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
)

// BuildHMACSignature 使用密钥对载荷进行签名创建HMAC签名
// 与Python实现保持一致：build_hmac_signature
//
// 参数:
//   - secret: Base64编码的密钥
//   - timestamp: 请求时间戳（字符串格式）
//   - method: HTTP方法（如 "POST", "GET"）
//   - requestPath: 请求路径（如 "/submit"）
//   - body: 请求体，可以是：
//   - string: JSON字符串（保留原始结构体的字段顺序，推荐）
//   - json.RawMessage: JSON字符串（保留字段顺序）
//   - struct: 将被序列化为JSON（保留结构体字段顺序）
//   - nil: 无请求体
//
// 重要提示：为匹配Python的HMAC签名，body应该是：
//   - JSON字符串（来自结构体的json.Marshal）- 保留字段顺序
//   - 结构体（将被序列化）- 保留结构体字段顺序
//   - 不要使用 map[string]interface{} - Go会按字母顺序排序map键！
func BuildHMACSignature(secret, timestamp, method, requestPath string, body interface{}) (string, error) {
	// Decode base64 secret (Python: base64.urlsafe_b64decode)
	secretBytes, err := base64.URLEncoding.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("failed to decode secret: %w", err)
	}

	// Build message: timestamp + method + requestPath
	// Python: message = str(timestamp) + str(method) + str(request_path)
	message := timestamp + method + requestPath

	if body != nil {
		var bodyJSONStr string

		// Python version: str(body).replace("'", '"')
		// Python's flow:
		//   1. dumps(body) -> JSON string (maintains insertion order)
		//   2. LocalSigner parses JSON string back to dict (maintains order in Python 3.7+)
		//   3. str(body).replace("'", '"') -> string with double quotes (maintains order)
		//
		// IMPORTANT: Go's json.Marshal behavior:
		//   - On maps: sorts keys alphabetically (causes HMAC mismatch!)
		//   - On structs: preserves field order (matches Python's behavior)
		//   - On strings/json.RawMessage: If body is already a JSON string, use it directly to preserve order
		//
		// Solution:
		//   - If body is a string or json.RawMessage (JSON), use it directly (preserves order from original struct)
		//   - If body is a struct, marshal it (preserves field order)
		//   - If body is a map, this will cause issues - should use struct instead

		switch v := body.(type) {
		case string:
			// Body is already a JSON string (from LocalSigner.SignRequest)
			// This preserves the field order from the original struct
			bodyJSONStr = v
		case json.RawMessage:
			// Body is json.RawMessage (JSON string)
			bodyJSONStr = string(v)
		case *json.RawMessage:
			// Body is pointer to json.RawMessage
			if v != nil {
				bodyJSONStr = string(*v)
			}
		default:
			// Body is struct or map - marshal it
			bodyJSON, err := json.Marshal(body)
			if err != nil {
				return "", fmt.Errorf("failed to marshal body: %w", err)
			}
			bodyJSONStr = string(bodyJSON)
		}

		// Go's json.Marshal produces compact JSON: {"key":"value","key2":"value2"}
		// Python's str(dict).replace("'", '"') produces: {"key": "value", "key2": "value2"} (with spaces)
		// We need to add spaces to match Python's format exactly
		// Add space after colon: "key":"value" -> "key": "value"
		// Use regex to replace ": with ":  (one space) only if not already followed by space
		bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)

		// Add space after comma: "a","b" -> "a", "b"
		// Match comma followed by quote (no space) and add space
		bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)

		// Also handle comma followed by { or [ (for nested structures)
		bodyJSONStr = regexp.MustCompile(`,(\{|\[)`).ReplaceAllString(bodyJSONStr, `, $1`)

		message += bodyJSONStr
	}

	// Create HMAC (Python: hmac.new(base64_secret, bytes(message, "utf-8"), hashlib.sha256))
	h := hmac.New(sha256.New, secretBytes)
	h.Write([]byte(message))
	digest := h.Sum(nil)

	// Base64 URL-safe encode (Python: base64.urlsafe_b64encode(h.digest()).decode("utf-8"))
	signature := base64.URLEncoding.EncodeToString(digest)
	return signature, nil
}
