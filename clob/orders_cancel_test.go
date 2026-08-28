package clob

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const cancelTestPrivateKey = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

func newCancelTestClient(t *testing.T, baseURL string) *orderClientImpl {
	t.Helper()
	w3, err := web3.NewClient(cancelTestPrivateKey, types.EOASignatureType, types.Polygon, baseURL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(w3.Close)
	base := &baseClient{
		baseURL:     baseURL,
		deriveCreds: &types.ApiCreds{Key: "test-key", Secret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", Passphrase: "test-passphrase"},
		web3Client:  w3,
	}
	return &orderClientImpl{baseClient: base}
}

func TestCancelMarketOrdersFiltersUseOfficialWireFields(t *testing.T) {
	conditionID, err := types.ParseConditionID("0x" + strings.Repeat("11", 32))
	if err != nil {
		t.Fatal(err)
	}
	tokenID, err := types.ParseTokenID("42")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		params     types.CancelMarketOrdersParams
		legacy     bool
		wantMarket string
		wantToken  string
	}{
		{name: "condition only", params: types.CancelMarketOrdersParams{ConditionID: conditionID}, wantMarket: conditionID.String()},
		{name: "token only", params: types.CancelMarketOrdersParams{TokenID: tokenID}, wantToken: tokenID.String()},
		{name: "condition and token", params: types.CancelMarketOrdersParams{ConditionID: conditionID, TokenID: tokenID}, wantMarket: conditionID.String(), wantToken: tokenID.String()},
		{name: "legacy wrapper", legacy: true, wantMarket: conditionID.String()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodDelete || r.URL.Path != "/cancel-market-orders" {
					http.Error(w, "unexpected endpoint", http.StatusBadRequest)
					return
				}
				if r.Header.Get("POLY_SIGNATURE") == "" {
					http.Error(w, "missing auth", http.StatusUnauthorized)
					return
				}
				var body map[string]string
				if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
					http.Error(w, decodeErr.Error(), http.StatusBadRequest)
					return
				}
				if body["market"] != tt.wantMarket || body["asset_id"] != tt.wantToken {
					t.Errorf("body = %#v, want market=%q asset_id=%q", body, tt.wantMarket, tt.wantToken)
				}
				if _, exists := body["condition_id"]; exists {
					t.Errorf("body contains obsolete condition_id: %#v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{}`))
			}))
			defer server.Close()

			client := newCancelTestClient(t, server.URL)
			var response *types.OrderCancelResponse
			var callErr error
			if tt.legacy {
				response, callErr = client.CancelMarketOrders(types.Keccak256(conditionID.String()))
			} else {
				response, callErr = client.CancelMarketOrdersByFilter(tt.params)
			}
			if callErr != nil {
				t.Fatalf("cancel: %v", callErr)
			}
			if calls.Load() != 1 {
				t.Fatalf("HTTP calls = %d, want 1", calls.Load())
			}
			if response == nil || response.Canceled == nil || response.NotCanceled == nil {
				t.Fatalf("response not normalized: %#v", response)
			}
		})
	}
}

func TestCancelMarketOrdersRejectsEmptyAndInvalidLegacyFilter(t *testing.T) {
	client := &orderClientImpl{baseClient: &baseClient{}}
	if _, err := client.CancelMarketOrdersByFilter(types.CancelMarketOrdersParams{}); err == nil || !strings.Contains(err.Error(), "requires ConditionID") {
		t.Fatalf("empty filter error = %v", err)
	}
	if _, err := client.CancelMarketOrders(types.Keccak256("invalid")); err == nil || !strings.Contains(err.Error(), "invalid condition ID") {
		t.Fatalf("legacy invalid condition error = %v", err)
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	authenticated := newCancelTestClient(t, server.URL)
	if _, err := authenticated.CancelMarketOrdersByFilter(types.CancelMarketOrdersParams{}); err == nil || !strings.Contains(err.Error(), "requires ConditionID") {
		t.Fatalf("empty filter error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("empty filter made %d HTTP requests", calls.Load())
	}
}

func TestCancelOrdersValidatesBoundaryAndCanonicalizesIDs(t *testing.T) {
	valid := types.Keccak256("0x" + strings.Repeat("ab", 32))
	unprefixed := types.Keccak256(strings.TrimPrefix(valid.String(), "0x"))

	client := &orderClientImpl{baseClient: &baseClient{}}
	empty, err := client.CancelOrders(nil)
	if err != nil || empty == nil || empty.Canceled == nil || empty.NotCanceled == nil {
		t.Fatalf("empty response=%#v err=%v", empty, err)
	}
	tooMany := make([]types.Keccak256, MaxCancelOrdersPerRequest+1)
	if _, err := client.CancelOrders(tooMany); err == nil || !strings.Contains(err.Error(), "exceeds per-request limit 3000") {
		t.Fatalf("too many error = %v", err)
	}
	if _, err := client.CancelOrders([]types.Keccak256{"invalid"}); err == nil || !strings.Contains(err.Error(), "index 0") {
		t.Fatalf("invalid ID error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []string
		if decodeErr := json.NewDecoder(r.Body).Decode(&body); decodeErr != nil {
			t.Errorf("decode body: %v", decodeErr)
			http.Error(w, decodeErr.Error(), http.StatusBadRequest)
			return
		}
		if len(body) != MaxCancelOrdersPerRequest {
			t.Errorf("body length = %d, want %d", len(body), MaxCancelOrdersPerRequest)
		}
		for i, orderID := range body {
			if orderID != valid.String() {
				t.Errorf("body[%d] = %q, want %q", i, orderID, valid.String())
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"canceled":null,"not_canceled":null}`))
	}))
	defer server.Close()
	authenticated := newCancelTestClient(t, server.URL)
	atLimit := make([]types.Keccak256, MaxCancelOrdersPerRequest)
	for i := range atLimit {
		atLimit[i] = unprefixed
	}
	response, err := authenticated.CancelOrders(atLimit)
	if err != nil {
		t.Fatalf("CancelOrders at limit: %v", err)
	}
	if response == nil || response.Canceled == nil || response.NotCanceled == nil {
		t.Fatalf("response not normalized: %#v", response)
	}
}
