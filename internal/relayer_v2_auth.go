// V2 Relayer API key 获取与认证。
//
// Polymarket 在 2026 切到 V2 relayer 协议后弃用了 POLY_BUILDER_* HMAC 五件套，
// 改用一对明文头：
//
//	RELAYER_API_KEY:         <uuid>
//	RELAYER_API_KEY_ADDRESS: <EOA 地址>
//
// V2 key 通过 SIWE（EIP-4361）派生：
//
//  1. GET  gamma-api.polymarket.com/nonce
//  2. EIP-191 personal_sign 一段 SIWE 文本
//  3. GET  gamma-api.polymarket.com/login (Bearer base64(JSON:::sig)) → session cookie
//  4. GET  relayer-v2.polymarket.com/relayer/api/keys (cookie) → 现存 key 列表
//     非空复用最新一条；空则
//  5. POST relayer-v2.polymarket.com/relayer/api/auth (cookie) → 新 mint 一条
//
// 命中本地缓存（~/.polymarket/relayer_key_<EOA>.json）+ GET /keys 验活时
// 整个 1~5 都跳过，无私钥操作。
package internal

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

const (
	// GammaAPIURL gamma-api 域，提供 /nonce + /login（SIWE）。
	GammaAPIURL = "https://gamma-api.polymarket.com"
	// RelayerV2URL relayer-v2 域，提供 /relayer/api/{auth,keys} 与业务 /submit 等。
	RelayerV2URL = "https://relayer-v2.polymarket.com"

	// V2 业务请求的两个明文头名。
	V2HeaderAPIKey  = "RELAYER_API_KEY"
	V2HeaderAddress = "RELAYER_API_KEY_ADDRESS"

	siweChainID    = 137
	siweStatement  = "Welcome to Polymarket! Sign to connect."
	siweDomain     = "polymarket.com"
	siweURI        = "https://polymarket.com"
	siweVersion    = "1"
	siweExpiration = 7 * 24 * time.Hour
)

// V2RelayerKey 是 relayer-v2 返回的 V2 API key 单条记录。
type V2RelayerKey struct {
	Key       string `json:"apiKey"`
	Address   string `json:"address"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// Headers 返回业务请求需要的两个明文头。
func (k V2RelayerKey) Headers() map[string]string {
	return map[string]string{
		V2HeaderAPIKey:  k.Key,
		V2HeaderAddress: k.Address,
	}
}

// EnsureV2RelayerKey 按 cache → SIWE/list → SIWE/mint 顺序拿一把可用的 V2 key。
//
// priv 必须能 personal_sign（用于 SIWE login），即调用者的 EOA 私钥。
// 同一 EOA 在进程内会被互斥串行化，避免并发首次 SIWE 时重复 mint。
//
// 入参 cachePath 为 "" 时禁用缓存（每次都跑 SIWE）。
func EnsureV2RelayerKey(ctx context.Context, priv *ecdsa.PrivateKey, cachePath string) (*V2RelayerKey, error) {
	if priv == nil {
		return nil, fmt.Errorf("priv key required")
	}
	eoa := ethcrypto.PubkeyToAddress(priv.PublicKey).Hex()

	if cachePath == "" {
		cachePath = DefaultV2KeyCachePath(eoa)
	}

	mu := v2KeyMutexFor(eoa)
	mu.Lock()
	defer mu.Unlock()

	if k, ok := loadV2KeyCache(cachePath); ok {
		if alive, _ := pingV2Key(ctx, k); alive {
			return &k, nil
		}
		// 缓存失效，往下走 SIWE
	}

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second}

	if err := siweLogin(ctx, c, priv, eoa); err != nil {
		return nil, fmt.Errorf("siwe login: %w", err)
	}

	existing, err := listV2KeysWithCookie(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}

	var picked V2RelayerKey
	if len(existing) > 0 {
		sort.Slice(existing, func(i, j int) bool { return existing[i].CreatedAt > existing[j].CreatedAt })
		picked = existing[0]
	} else {
		k, err := mintV2Key(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("mint key: %w", err)
		}
		picked = k
	}

	if cachePath != "" {
		_ = saveV2KeyCache(cachePath, picked) // 缓存写失败不致命
	}
	return &picked, nil
}

// DefaultV2KeyCachePath 返回 ~/.polymarket/relayer_key_<eoa-lower>.json。
func DefaultV2KeyCachePath(eoa string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".polymarket", fmt.Sprintf("relayer_key_%s.json", strings.ToLower(eoa)))
}

// --------- 内部：cache + http helpers ---------

var (
	v2KeyMuMap   = map[string]*sync.Mutex{}
	v2KeyMuGuard sync.Mutex
)

func v2KeyMutexFor(eoa string) *sync.Mutex {
	key := strings.ToLower(eoa)
	v2KeyMuGuard.Lock()
	defer v2KeyMuGuard.Unlock()
	if m, ok := v2KeyMuMap[key]; ok {
		return m
	}
	m := &sync.Mutex{}
	v2KeyMuMap[key] = m
	return m
}

func loadV2KeyCache(path string) (V2RelayerKey, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return V2RelayerKey{}, false
	}
	var k V2RelayerKey
	if json.Unmarshal(b, &k) != nil || k.Key == "" || k.Address == "" {
		return V2RelayerKey{}, false
	}
	return k, true
}

func saveV2KeyCache(path string, k V2RelayerKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(k, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

func pingV2Key(ctx context.Context, k V2RelayerKey) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, RelayerV2URL+"/relayer/api/keys", nil)
	for h, v := range k.Headers() {
		req.Header.Set(h, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == 200, nil
}

func listV2KeysWithCookie(ctx context.Context, c *http.Client) ([]V2RelayerKey, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, RelayerV2URL+"/relayer/api/keys", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var out []V2RelayerKey
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode %q: %w", body, err)
	}
	return out, nil
}

func mintV2Key(ctx context.Context, c *http.Client) (V2RelayerKey, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, RelayerV2URL+"/relayer/api/auth", bytes.NewReader([]byte("{}")))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return V2RelayerKey{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return V2RelayerKey{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var k V2RelayerKey
	if err := json.Unmarshal(body, &k); err != nil {
		return V2RelayerKey{}, err
	}
	return k, nil
}

// --------- SIWE ---------

type siweMsg struct {
	Domain         string `json:"domain"`
	Address        string `json:"address"`
	Statement      string `json:"statement"`
	URI            string `json:"uri"`
	Version        string `json:"version"`
	ChainID        int64  `json:"chainId"`
	Nonce          string `json:"nonce"`
	IssuedAt       string `json:"issuedAt"`
	ExpirationTime string `json:"expirationTime"`
}

func siweLogin(ctx context.Context, c *http.Client, priv *ecdsa.PrivateKey, eoaHex string) error {
	// 1) nonce
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, GammaAPIURL+"/nonce", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("nonce HTTP %d: %s", resp.StatusCode, body)
	}
	var n struct {
		Nonce string `json:"nonce"`
	}
	if err := json.Unmarshal(body, &n); err != nil || n.Nonce == "" {
		return fmt.Errorf("decode nonce body=%q err=%v", body, err)
	}

	// 2) build SIWE & personal_sign
	now := time.Now().UTC()
	msg := siweMsg{
		Domain:         siweDomain,
		Address:        eoaHex,
		Statement:      siweStatement,
		URI:            siweURI,
		Version:        siweVersion,
		ChainID:        siweChainID,
		Nonce:          n.Nonce,
		IssuedAt:       now.Format(time.RFC3339),
		ExpirationTime: now.Add(siweExpiration).Format(time.RFC3339),
	}
	plain := siweString(msg)
	hash := accounts.TextHash([]byte(plain))
	sig, err := ethcrypto.Sign(hash, priv)
	if err != nil {
		return fmt.Errorf("personal_sign: %w", err)
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	jsonFields, _ := json.Marshal(msg)
	bearer := base64.StdEncoding.EncodeToString([]byte(string(jsonFields) + ":::0x" + hex.EncodeToString(sig)))

	// 3) GET /login with bearer → Set-Cookie
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, GammaAPIURL+"/login", nil)
	req2.Header.Set("Accept", "application/json")
	req2.Header.Set("Authorization", "Bearer "+bearer)
	resp2, err := c.Do(req2)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	rb, _ := io.ReadAll(resp2.Body)
	if resp2.StatusCode/100 != 2 {
		return fmt.Errorf("login HTTP %d: %s", resp2.StatusCode, rb)
	}
	return nil
}

func siweString(m siweMsg) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s wants you to sign in with your Ethereum account:\n", m.Domain)
	fmt.Fprintf(&b, "%s\n\n", m.Address)
	if m.Statement != "" {
		fmt.Fprintf(&b, "%s\n\n", m.Statement)
	}
	fmt.Fprintf(&b, "URI: %s\n", m.URI)
	fmt.Fprintf(&b, "Version: %s\n", m.Version)
	fmt.Fprintf(&b, "Chain ID: %d\n", m.ChainID)
	fmt.Fprintf(&b, "Nonce: %s\n", m.Nonce)
	fmt.Fprintf(&b, "Issued At: %s", m.IssuedAt)
	if m.ExpirationTime != "" {
		fmt.Fprintf(&b, "\nExpiration Time: %s", m.ExpirationTime)
	}
	return b.String()
}
