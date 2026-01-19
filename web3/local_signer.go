package web3

import (
	"encoding/json"
	"fmt"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

// LocalSigner is a local signing service that replaces the external builder-signing-server
//
// It implements the same signing functionality as the external service, supporting:
// - Level 1 signing: EIP-712 signing with private key (when no builder_creds)
// - Level 2 signing: HMAC signing with API credentials (when builder_creds provided)
type LocalSigner struct {
	signer       *signing.Signer
	builderCreds *types.ApiCreds
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

	// Choose signing method based on whether builder_creds exists
	if ls.builderCreds != nil {
		// Use Level 2 signing (HMAC, requires builder_creds)
		headers, err := internal.CreateLevel2Headers(
			ls.signer,
			ls.builderCreds,
			requestArgs,
			true, // builder=true
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create level 2 headers: %w", err)
		}
		return headers, nil
	}

	// Use Level 1 signing (EIP-712, based on private key)
	// Note: Level 1 signing usually doesn't need body, but we handle it for compatibility
	var nonce *int
	headers, err := internal.CreateLevel1Headers(ls.signer, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to create level 1 headers: %w", err)
	}
	return headers, nil
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
