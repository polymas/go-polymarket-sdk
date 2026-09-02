// relayer_v2_submit_probe：验证 V2 头是否被 relayer 的写端点（POST /submit）接受。
//
// 判定逻辑：对每个端点发两次请求 — 一次无认证、一次带 V2 头。
//
//	无认证返回 401  → baseline
//	V2 头返回 401   → ❌ V2 不通过 → 不要换协议
//	V2 头返回其它   → ✅ V2 头被认证层接受（业务层可能因 body 无效再拒）
//
// 用法：
//
//	APIKEY=<uuid> ADDR=0x... go run ./cmd/diagnostics/relayer-v2-submit-probe
package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const relayerURL = "https://relayer-v2.polymarket.com"

type probe struct {
	method string
	path   string
	body   string
}

func main() {
	apiKey := strings.TrimSpace(os.Getenv("APIKEY"))
	addr := strings.TrimSpace(os.Getenv("ADDR"))
	if apiKey == "" || addr == "" {
		log.Fatal("env APIKEY and ADDR are required\n  APIKEY=019e5a01-... ADDR=0x0cd5... go run ./cmd/diagnostics/relayer-v2-submit-probe")
	}

	v2 := map[string]string{
		"RELAYER_API_KEY":         apiKey,
		"RELAYER_API_KEY_ADDRESS": addr,
	}

	probes := []probe{
		// 写端点（业务核心）— body 故意发空对象
		{"POST", "/submit", "{}"},
		// 已知通的读端点 — 作为 V2 头自检（应 200）
		{"GET", "/transactions", ""},
		// 也试一下另一个常见写端点
		{"POST", "/relay-payload", "{}"},
	}

	c := &http.Client{Timeout: 15 * time.Second}
	fmt.Printf("%-7s %-22s | %-12s | %-12s | verdict\n", "METHOD", "PATH", "no-auth", "V2-headers")
	fmt.Println(strings.Repeat("-", 80))

	for _, p := range probes {
		noAuth := hit(c, p, nil)
		withV2 := hit(c, p, v2)
		verdict := classify(noAuth, withV2)
		fmt.Printf("%-7s %-22s | %-12s | %-12s | %s\n",
			p.method, p.path, noAuth, withV2, verdict)
	}

	fmt.Println()
	fmt.Println("Legend:")
	fmt.Println("  ✅ V2 OK    = V2 头被认证层接受（body 是否合法另说）")
	fmt.Println("  ❌ V2 FAIL  = V2 头也被认证层拒绝，方案破产")
	fmt.Println("  ⚠️  AMBIG    = 无认证状态意外（端点不存在/拒所有），判不出来")
}

func hit(c *http.Client, p probe, headers map[string]string) string {
	var body io.Reader
	if p.body != "" {
		body = bytes.NewReader([]byte(p.body))
	}
	req, _ := http.NewRequest(p.method, relayerURL+p.path, body)
	if p.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return "ERR:" + err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	snippet := strings.TrimSpace(string(b))
	if len(snippet) > 40 {
		snippet = snippet[:37] + "..."
	}
	if snippet == "" {
		return fmt.Sprintf("%d", resp.StatusCode)
	}
	return fmt.Sprintf("%d %s", resp.StatusCode, snippet)
}

func classify(noAuth, withV2 string) string {
	noAuthIs401 := strings.HasPrefix(noAuth, "401")
	v2Is401 := strings.HasPrefix(withV2, "401")
	switch {
	case noAuthIs401 && !v2Is401:
		return "✅ V2 OK (auth layer passed)"
	case noAuthIs401 && v2Is401:
		return "❌ V2 FAIL (still 401 with V2 headers)"
	default:
		return "⚠️  AMBIG (no-auth not 401)"
	}
}
