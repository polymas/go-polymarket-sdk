package web3

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

// LocalSigner is a local signing service that replaces the external builder-signing-server
//
// 三种鉴权路径（按优先级）：
//   - V2 RelayerKey（推荐）：注入 v2Key 后所有 /relayer 请求只用 RELAYER_API_KEY +
//     RELAYER_API_KEY_ADDRESS 两个明文头，对应 polymarket 2026 新 relayer 协议。
//   - L2 POLY_BUILDER_* HMAC（已弃用）：传 builderCreds 走旧协议，仅作 fallback。
//   - L1 EIP-712 (POLY_*)：无 creds 时走 CLOB ClobAuth 派生头，给非-relayer 用途。
type LocalSigner struct {
	mu           sync.RWMutex
	signer       *signing.Signer
	builderCreds *types.ApiCreds
	v2Key        *internal.V2RelayerKey
	closed       bool
}

// NewLocalSigner creates a new LocalSigner instance
//
// Args:
//   - signer: Signer instance containing private key and chain ID
//   - builderCreds: Optional API credentials, if provided uses Level 2 signing
func NewLocalSigner(signer *signing.Signer, builderCreds *types.ApiCreds) *LocalSigner {
	return &LocalSigner{
		signer:       signer,
		builderCreds: builderCreds,
	}
}

// SignPayload signs a request payload and returns signed headers
//
// This method mimics the behavior of the external service:
// - Receives a payload containing method, path, body
// - Returns signed headers
//
// Args:
//   - payload: Map containing:
//   - method: HTTP method (e.g., "POST", "GET", "DELETE")
//   - path: Request path (e.g., "/submit")
//   - body: Request body (can be map, struct, or JSON string)
//
// Returns:
//   - Map containing signed header information
//
// Errors:
//   - Returns error if payload format is incorrect
func (ls *LocalSigner) SignPayload(payload map[string]interface{}) (map[string]string, error) {
	ls.mu.RLock()
	if ls.closed {
		ls.mu.RUnlock()
		return nil, signing.ErrSignerClosed
	}
	signer := ls.signer
	builderCreds := ls.builderCreds
	var v2Key *internal.V2RelayerKey
	if ls.v2Key != nil {
		copyKey := *ls.v2Key
		v2Key = &copyKey
	}
	ls.mu.RUnlock()

	// Validate payload format
	if payload == nil {
		return nil, fmt.Errorf("payload must not be nil")
	}

	method, ok := payload["method"].(string)
	if !ok || method == "" {
		return nil, fmt.Errorf("payload must contain 'method' field as string")
	}

	path, ok := payload["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("payload must contain 'path' field as string")
	}

	// Convert body to RequestBody (preserves field order for HMAC signature)
	var requestBody *types.RequestBody
	if body := payload["body"]; body != nil {
		if bodyStr, ok := body.(string); ok {
			rb := types.RequestBody(bodyStr)
			requestBody = &rb
		} else {
			bodyJSON, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal body: %w", err)
			}
			rb := types.RequestBody(bodyJSON)
			requestBody = &rb
		}
	}

	// Build RequestArgs
	requestArgs := &types.RequestArgs{
		Method:      method,
		RequestPath: path,
		Body:        requestBody,
	}

	// V2 优先：注入后直接返回明文双头，无 HMAC 无私钥。
	if v2Key != nil && v2Key.Key != "" {
		return v2Key.Headers(), nil
	}

	// Relayer uses POLY_BUILDER_* HMAC headers when creds are present,
	// otherwise falls back to Level 1 EIP-712 signing with the private key.
	if builderCreds != nil {
		var body interface{}
		if requestArgs.Body != nil {
			body = string(*requestArgs.Body)
		}
		headers, err := internal.CreateRelayerHeaders(builderCreds, requestArgs, body)
		if err != nil {
			return nil, fmt.Errorf("failed to create relayer headers: %w", err)
		}
		return headers, nil
	}

	var nonce *int
	headers, err := internal.CreateLevel1Headers(signer, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to create level 1 headers: %w", err)
	}
	return headers, nil
}

// SetV2Key 注入 V2 relayer key。注入后所有 SignPayload 调用返回 V2 明文双头。
// 传 nil 解除注入，回到旧 builderCreds / L1 路径。
func (ls *LocalSigner) SetV2Key(k *internal.V2RelayerKey) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.closed {
		return
	}
	ls.v2Key = k
}

// HasV2Key 报告当前是否已注入可用的 V2 relayer key。
func (ls *LocalSigner) HasV2Key() bool {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.v2Key != nil && ls.v2Key.Key != ""
}

// Clear 释放 LocalSigner 持有的签名器与 Relayer 凭证引用。
// 字符串历史副本无法由 Go 可靠擦除，因此这里只提供尽力清理。
func (ls *LocalSigner) Clear() {
	if ls == nil {
		return
	}
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if ls.v2Key != nil {
		ls.v2Key.Key = ""
		ls.v2Key.Address = ""
		ls.v2Key.CreatedAt = ""
		ls.v2Key.UpdatedAt = ""
	}
	ls.v2Key = nil
	ls.builderCreds = nil
	ls.signer = nil
	ls.closed = true
}

// SignRequest is a convenience method that directly uses method, path, body parameters for signing
//
// Args:
//   - method: HTTP method
//   - path: Request path
//   - body: Request body (optional, can be nil)
//   - struct: Will be marshaled to JSON (preserves struct field order, recommended)
//   - nil: No body
//
// Returns:
//   - Map containing signed header information
//
// IMPORTANT: body should be a struct (not map) to preserve field order for HMAC signature matching
func (ls *LocalSigner) SignRequest(method, path string, body interface{}) (map[string]string, error) {
	payload := map[string]interface{}{
		"method": method,
		"path":   path,
	}

	if body != nil {
		// Marshal body to JSON string (matching Python's dumps behavior)
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		payload["body"] = string(bodyJSON)
	}

	return ls.SignPayload(payload)
}

// CreateLocalSigner is a factory function to create a LocalSigner instance
func CreateLocalSigner(
	privateKey string,
	chainID types.ChainID,
	builderCreds *types.ApiCreds,
) (*LocalSigner, error) {
	signer, err := signing.NewSigner(privateKey, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	return NewLocalSigner(signer, builderCreds), nil
}
