package web3

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

func TestCollateralAndLegacyUSDCEBalancesUseDifferentContracts(t *testing.T) {
	const (
		privateKey = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
		wallet     = types.EthAddress("0x1111111111111111111111111111111111111111")
	)

	var (
		mu      sync.Mutex
		targets []string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage   `json:"id"`
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if request.Method != "eth_call" || len(request.Params) == 0 {
			writeRPCError(w, request.ID, "unexpected RPC method")
			return
		}
		var call struct {
			To    string `json:"to"`
			Data  string `json:"data"`
			Input string `json:"input"`
		}
		if err := json.Unmarshal(request.Params[0], &call); err != nil {
			writeRPCError(w, request.ID, err.Error())
			return
		}
		calldata := call.Data
		if calldata == "" {
			calldata = call.Input
		}
		if !strings.HasPrefix(strings.ToLower(calldata), "0x70a08231") {
			writeRPCError(w, request.ID, "unexpected calldata")
			return
		}

		mu.Lock()
		targets = append(targets, call.To)
		mu.Unlock()

		var raw int64
		switch {
		case strings.EqualFold(call.To, internal.PolygonPUSD):
			raw = 51_144_933
		case strings.EqualFold(call.To, internal.PolygonCollateral):
			raw = 7_000_000
		default:
			writeRPCError(w, request.ID, "unexpected contract target")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x%064x"}`, request.ID, raw)
	}))
	defer server.Close()

	client, err := NewClient(privateKey, types.EOASignatureType, types.Polygon, server.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	collateral, err := client.GetCollateralBalance(wallet)
	if err != nil {
		t.Fatalf("GetCollateralBalance: %v", err)
	}
	if collateral != 51.144933 {
		t.Fatalf("GetCollateralBalance = %.6f, want 51.144933", collateral)
	}

	usdcE, err := client.GetUSDCEBalance(wallet)
	if err != nil {
		t.Fatalf("GetUSDCEBalance: %v", err)
	}
	if usdcE != 7 {
		t.Fatalf("GetUSDCEBalance = %.6f, want 7", usdcE)
	}

	legacy, err := client.GetUSDCBalance(wallet)
	if err != nil {
		t.Fatalf("deprecated GetUSDCBalance: %v", err)
	}
	if legacy != usdcE {
		t.Fatalf("deprecated GetUSDCBalance = %.6f, want USDC.e %.6f", legacy, usdcE)
	}

	mu.Lock()
	gotTargets := append([]string(nil), targets...)
	mu.Unlock()
	wantTargets := []string{internal.PolygonPUSD, internal.PolygonCollateral, internal.PolygonCollateral}
	if len(gotTargets) != len(wantTargets) {
		t.Fatalf("eth_call targets = %v, want %v", gotTargets, wantTargets)
	}
	for i := range wantTargets {
		if !strings.EqualFold(gotTargets[i], wantTargets[i]) {
			t.Errorf("eth_call target[%d] = %s, want %s", i, gotTargets[i], wantTargets[i])
		}
	}

	if _, err := client.GetCollateralBalance(types.EthAddress("not-an-address")); err == nil {
		t.Fatal("GetCollateralBalance with invalid address returned nil error")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(targets) != len(wantTargets) {
		t.Fatalf("invalid address caused an RPC call: targets = %v", targets)
	}
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, message string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32602,"message":%q}}`, id, message)
}
