// relayer_apikey_mint：get-or-mint Polymarket V2 relayer API key，带本地缓存。
//
// 策略（按优先级回退）：
//
//  1. 本地缓存 ~/.polymarket/relayer_key_<EOA>.json
//     有 → 用 V2 头打 GET /relayer/api/keys 验活
//          200 → 直接复用，无私钥操作 ✅
//          401 → 缓存失效，落到 step 2
//
//  2. SIWE login（要私钥 personal_sign 一次）
//     GET /relayer/api/keys (cookie auth) → 返回该 EOA 名下全部 key
//     非空 → 取 createdAt 最新一条，复用
//     空   → POST /relayer/api/auth mint 新 key
//
//  3. 拿到 (apiKey, address) 写回缓存，下次走 step 1
//
// 业务请求只用两个明文头（无 HMAC、无 secret、无 timestamp 签名）：
//
//	RELAYER_API_KEY:         <apiKey>
//	RELAYER_API_KEY_ADDRESS: <EOA>
//
// 用法：
//
//	PK=0x... go run ./examples/relayer_apikey_mint
//	PK=0x... CACHE=0 go run ./examples/relayer_apikey_mint   # 跳过本地缓存
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/accounts"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
)

const (
	gammaURL   = "https://gamma-api.polymarket.com"
	relayerURL = "https://relayer-v2.polymarket.com"
	chainID    = 137
)

type v2Key struct {
	Key       string `json:"apiKey"`
	Address   string `json:"address"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func (k v2Key) headers() map[string]string {
	return map[string]string{
		"RELAYER_API_KEY":         k.Key,
		"RELAYER_API_KEY_ADDRESS": k.Address,
	}
}

func main() {
	pk := strings.TrimSpace(os.Getenv("PK"))
	if pk == "" {
		log.Fatal("env PK is required (EOA private key, 0x...)")
	}
	priv, err := ethcrypto.HexToECDSA(strings.TrimPrefix(pk, "0x"))
	if err != nil {
		log.Fatalf("bad PK: %v", err)
	}
	eoa := ethcrypto.PubkeyToAddress(priv.PublicKey)
	useCache := os.Getenv("CACHE") != "0"

	fmt.Printf("EOA: %s\n", eoa.Hex())

	cachePath := cacheFile(eoa.Hex())
	var key v2Key
	var source string

	// Step 1: cache + ping
	if useCache {
		if cached, ok := loadCache(cachePath); ok {
			if alive, _ := pingKeys(cached); alive {
				key = cached
				source = "cache (verified alive)"
			} else {
				fmt.Println("cached key failed ping, falling back to SIWE...")
			}
		}
	}

	// Step 2: SIWE login → list/mint
	if key.Key == "" {
		jar, _ := cookiejar.New(nil)
		client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
		if err := siweLogin(client, priv, eoa.Hex()); err != nil {
			log.Fatalf("SIWE login: %v", err)
		}
		fmt.Println("SIWE login OK")

		existing, err := listKeysWithCookie(client)
		if err != nil {
			log.Fatalf("list keys: %v", err)
		}

		if len(existing) > 0 {
			sort.Slice(existing, func(i, j int) bool { return existing[i].CreatedAt > existing[j].CreatedAt })
			key = existing[0]
			source = fmt.Sprintf("listed (%d total, picked latest)", len(existing))
		} else {
			k, err := mintNewKey(client)
			if err != nil {
				log.Fatalf("mint: %v", err)
			}
			key = k
			source = "freshly minted"
		}

		if useCache {
			if err := saveCache(cachePath, key); err != nil {
				fmt.Printf("WARN: save cache: %v\n", err)
			}
		}
	}

	fmt.Println()
	fmt.Println("=== Polymarket V2 Relayer API Key ===")
	fmt.Printf("source:  %s\n", source)
	fmt.Printf("apiKey:  %s\n", key.Key)
	fmt.Printf("address: %s\n", key.Address)
	if key.CreatedAt != "" {
		fmt.Printf("created: %s\n", key.CreatedAt)
	}
	if useCache {
		fmt.Printf("cache:   %s\n", cachePath)
	}
	fmt.Println()
	fmt.Println("Headers for /relayer/* business calls:")
	fmt.Printf("  RELAYER_API_KEY:         %s\n", key.Key)
	fmt.Printf("  RELAYER_API_KEY_ADDRESS: %s\n", key.Address)

	if !strings.EqualFold(key.Address, eoa.Hex()) {
		fmt.Printf("\nWARN: returned address %s != local EOA %s\n", key.Address, eoa.Hex())
	}
}

// ------- cache -------

func cacheFile(eoa string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".polymarket", fmt.Sprintf("relayer_key_%s.json", strings.ToLower(eoa)))
}

func loadCache(path string) (v2Key, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return v2Key{}, false
	}
	var k v2Key
	if json.Unmarshal(b, &k) != nil || k.Key == "" || k.Address == "" {
		return v2Key{}, false
	}
	return k, true
}

func saveCache(path string, k v2Key) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(k, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

// ------- relayer probes -------

// pingKeys hits GET /relayer/api/keys using V2 headers; 200 = key alive.
func pingKeys(k v2Key) (bool, error) {
	req, _ := http.NewRequest(http.MethodGet, relayerURL+"/relayer/api/keys", nil)
	for h, v := range k.headers() {
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

func listKeysWithCookie(c *http.Client) ([]v2Key, error) {
	req, _ := http.NewRequest(http.MethodGet, relayerURL+"/relayer/api/keys", nil)
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
	var out []v2Key
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode %q: %w", body, err)
	}
	return out, nil
}

func mintNewKey(c *http.Client) (v2Key, error) {
	req, _ := http.NewRequest(http.MethodPost, relayerURL+"/relayer/api/auth", bytes.NewReader([]byte("{}")))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return v2Key{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return v2Key{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var k v2Key
	if err := json.Unmarshal(body, &k); err != nil {
		return v2Key{}, err
	}
	return k, nil
}

// ------- SIWE -------

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

func siweLogin(c *http.Client, priv *ecdsa.PrivateKey, eoaHex string) error {
	req, _ := http.NewRequest(http.MethodGet, gammaURL+"/nonce", nil)
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
		return fmt.Errorf("decode nonce: %v body=%s", err, body)
	}

	now := time.Now().UTC()
	msg := siweMsg{
		Domain:         "polymarket.com",
		Address:        eoaHex,
		Statement:      "Welcome to Polymarket! Sign to connect.",
		URI:            "https://polymarket.com",
		Version:        "1",
		ChainID:        chainID,
		Nonce:          n.Nonce,
		IssuedAt:       now.Format(time.RFC3339),
		ExpirationTime: now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
	}
	plain := siweString(msg)

	hash := accounts.TextHash([]byte(plain))
	sig, err := ethcrypto.Sign(hash, priv)
	if err != nil {
		return fmt.Errorf("sign: %w", err)
	}
	if sig[64] < 27 {
		sig[64] += 27
	}

	jsonFields, _ := json.Marshal(msg)
	bearer := base64.StdEncoding.EncodeToString([]byte(string(jsonFields) + ":::0x" + hex.EncodeToString(sig)))

	req2, _ := http.NewRequest(http.MethodGet, gammaURL+"/login", nil)
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
