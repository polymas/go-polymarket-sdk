package relayer

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCWIARelayBodyShape 锁死 CWIA /submit 请求体的 JSON 形状，确保字段名、
// 嵌套层级、字段顺序与官方 py-builder-relayer-client 中
// DepositWalletBatchRequest.to_dict() 输出完全一致。
//
// HMAC 签名以 body 字符串为输入，字段顺序错位会导致鉴权失败，所以这里用
// 顺序比对而不是结构比对。
func TestCWIARelayBodyShape(t *testing.T) {
	body := &CWIARelayBody{
		Type:      "WALLET",
		From:      "0xEOA",
		To:        "0xFACTORY",
		Nonce:     "0",
		Signature: "0xSIG",
		DepositWalletParams: CWIADepositWalletParams{
			DepositWallet: "0xWALLET",
			Deadline:      "1234",
			Calls: []CWIARelayCallPayload{
				{Target: "0xT", Value: "0", Data: "0xDA"},
			},
		},
	}
	out, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	// 期望严格按以下顺序输出（与官方 to_dict 一致）
	want := `{"type":"WALLET","from":"0xEOA","to":"0xFACTORY","nonce":"0","signature":"0xSIG","depositWalletParams":{"depositWallet":"0xWALLET","deadline":"1234","calls":[{"target":"0xT","value":"0","data":"0xDA"}]}}`

	if string(out) != want {
		t.Fatalf("CWIA relay body shape mismatch:\n  want = %s\n  got  = %s", want, string(out))
	}

	// Sanity：禁止再混入老 v1.10.0 的字段
	for _, banned := range []string{`"data":`, `"proxyWallet":`, `"metadata":`} {
		// data/proxyWallet 不应该出现在 top-level；calls 里的 data 是允许的
		if strings.Contains(string(out), banned) && banned != `"data":` {
			t.Fatalf("relay body contains stale field %s", banned)
		}
	}
}
