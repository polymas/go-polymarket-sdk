package relayer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/polymas/go-polymarket-sdk/errors"
	"github.com/polymas/go-polymarket-sdk/types"
)

// 公开测试私钥（anvil/hardhat 默认账户 #0），不持有任何真实资产。
const retryTestPK = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

// mockRPC 起一个极简 JSON-RPC server：eth_call 返回 32 字节零填充地址
// （够 proxy 地址派生用），eth_estimateGas 返回固定值。
func mockRPC(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		result := "0x1"
		switch req.Method {
		case "eth_call":
			result = "0x000000000000000000000000aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		case "eth_estimateGas":
			result = "0x186a0"
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"%s"}`, req.ID, result)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newRetryTestClient 构造一个 Proxy 类型 GaslessClient：
//   - RPC 指向 mock JSON-RPC（proxy 地址派生、estimateGas 都走它）
//   - relayer 指向给定 httptest server
//   - 通过环境变量禁用 V2 key 派生，避免测试触网 SIWE
func newRetryTestClient(t *testing.T, relayerURL string) *GaslessClient {
	t.Helper()
	t.Setenv("POLYMARKET_DISABLE_V2_RELAYER_KEY", "1")
	rpc := mockRPC(t)
	c, err := NewGaslessClient(retryTestPK, types.ProxySignatureType, types.Polygon, nil, rpc.URL)
	if err != nil {
		t.Fatalf("NewGaslessClient: %v", err)
	}
	c.relayURL = relayerURL
	return c
}

// mockRelayer 起一个假 relayer：/nonce 恒返回 0，/submit 按调用次序返回 responses。
func mockRelayer(t *testing.T, submitCount *int64, responses ...func(w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/nonce"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"nonce": "0"})
		case strings.HasPrefix(r.URL.Path, "/submit"):
			n := atomic.AddInt64(submitCount, 1)
			if int(n) > len(responses) {
				t.Errorf("unexpected extra /submit call #%d", n)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			responses[n-1](w)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func respJSON(status int, body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}
}

func testTxn() []map[string]interface{} {
	return []map[string]interface{}{{
		"typeCode": 1,
		"to":       "0x4D97DCd97eC945f40cF65F87097ACe5EA0476045",
		"value":    0,
		"data":     "0x",
	}}
}

// 预模拟 "batch would revert" 应在短暂等待后重建批次重试一次，
// 第二次的错误原样向上返回。
func TestExecuteGaslessBatch_RetriesOnWouldRevert(t *testing.T) {
	oldDelay := relayRevertRetryDelay
	relayRevertRetryDelay = 10 * time.Millisecond
	t.Cleanup(func() { relayRevertRetryDelay = oldDelay })

	var submits int64
	srv := mockRelayer(t, &submits,
		respJSON(http.StatusBadRequest, `{"error":"batch would revert: execution reverted"}`),
		respJSON(http.StatusBadRequest, `{"error":"second failure"}`),
	)
	c := newRetryTestClient(t, srv.URL)

	_, err := c.executeGaslessBatch(testTxn(), "test", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt64(&submits); got != 2 {
		t.Fatalf("expected 2 submit calls, got %d", got)
	}
	if !strings.Contains(err.Error(), "second failure") {
		t.Fatalf("expected final error from retry attempt, got: %v", err)
	}
}

// 401 invalid authorization 应作废 V2 key 后重试一次。
func TestExecuteGaslessBatch_RetriesOn401(t *testing.T) {
	var submits int64
	srv := mockRelayer(t, &submits,
		respJSON(http.StatusUnauthorized, `{"error":"invalid authorization"}`),
		respJSON(http.StatusBadRequest, `{"error":"second failure"}`),
	)
	c := newRetryTestClient(t, srv.URL)

	_, err := c.executeGaslessBatch(testTxn(), "test", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt64(&submits); got != 2 {
		t.Fatalf("expected 2 submit calls, got %d", got)
	}
}

// 其他 4xx 不应重试。
func TestExecuteGaslessBatch_NoRetryOnOtherErrors(t *testing.T) {
	var submits int64
	srv := mockRelayer(t, &submits,
		respJSON(http.StatusBadRequest, `{"error":"insufficient balance"}`),
	)
	c := newRetryTestClient(t, srv.URL)

	_, err := c.executeGaslessBatch(testTxn(), "test", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt64(&submits); got != 1 {
		t.Fatalf("expected exactly 1 submit call, got %d", got)
	}
	var httpErr *sdkerrors.RelayHTTPError
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusBadRequest {
		t.Fatalf("expected RelayHTTPError with 400, got: %v", err)
	}
}

func TestExecuteGaslessBatchContextTimeoutIsAmbiguousAndNotRetried(t *testing.T) {
	var submits int64
	srv := mockRelayer(t, &submits, func(w http.ResponseWriter) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"transactionID":"late","state":"STATE_NEW"}`))
	})
	c := newRetryTestClient(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := c.executeGaslessBatchContext(ctx, testTxn(), "test timeout", "test")
	if !sdkerrors.IsAmbiguousOutcome(err) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%T %v, want ambiguous deadline", err, err)
	}
	if got := atomic.LoadInt64(&submits); got != 1 {
		t.Fatalf("submit calls=%d, want exactly one", got)
	}
}

func TestRelayerHTTPErrorWrapsRedactedAPIError(t *testing.T) {
	var submits int64
	srv := mockRelayer(t, &submits, respJSON(http.StatusTooManyRequests,
		`{"error":{"code":"RATE_LIMIT","message":"slow down"},"signature":"0xsecret"}`))
	c := newRetryTestClient(t, srv.URL)
	_, err := c.executeGaslessBatch(testTxn(), "test", "test")
	var apiErr *sdkerrors.APIError
	if !errors.As(err, &apiErr) || apiErr.Service != "relayer" || apiErr.Status != http.StatusTooManyRequests || apiErr.ErrorCode != "RATE_LIMIT" {
		t.Fatalf("error=%T %v, want relayer APIError", err, err)
	}
	if strings.Contains(apiErr.RawResponse, "0xsecret") {
		t.Fatalf("response leaked signature: %s", apiErr.RawResponse)
	}
}

// getRelayNonce 对传输层错误（连接被中断产生的 EOF 等）应重试而不是立即失败。
func TestGetRelayNonce_RetriesOnTransportError(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&calls, 1) == 1 {
			// 第一次直接断开连接，客户端表现为 EOF / unexpected EOF
			panic(http.ErrAbortHandler)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"nonce": "7"})
	}))
	t.Cleanup(srv.Close)
	c := newRetryTestClient(t, srv.URL)

	nonce, err := c.getRelayNonce("WALLET")
	if err != nil {
		t.Fatalf("expected nonce after retry, got error: %v", err)
	}
	if nonce != 7 {
		t.Fatalf("expected nonce 7, got %d", nonce)
	}
	if got := atomic.LoadInt64(&calls); got != 2 {
		t.Fatalf("expected 2 nonce calls, got %d", got)
	}
}
