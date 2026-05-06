package web3

import (
	"encoding/json"
	"testing"
)

// TestCWIADeployBodyShape 锁 WALLET-CREATE 请求体 schema：与官方
// py-builder-relayer-client/models.py:DepositWalletCreateRequest.to_dict() 一致。
func TestCWIADeployBodyShape(t *testing.T) {
	body := &CWIADeployBody{
		Type: "WALLET-CREATE",
		From: "0xEOA",
		To:   "0xFACTORY",
	}
	out, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"WALLET-CREATE","from":"0xEOA","to":"0xFACTORY"}`
	if string(out) != want {
		t.Fatalf("WALLET-CREATE body mismatch:\n  want = %s\n  got  = %s", want, string(out))
	}
}
