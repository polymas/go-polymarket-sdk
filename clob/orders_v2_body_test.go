package clob

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestV2OrderBodyShape 锁定 V2 提交体的字段顺序与新增可选字段的位置。
//
// HMAC 签名以 body 字符串为输入，字段顺序错位 → 鉴权失败；
// 字段缺失 → 后端 422 / 校验拒单。
//
// 顺序对照 py-clob-client-v2 order_to_json_v2()：
//
//	order: { salt, maker, signer, tokenId, makerAmount, takerAmount, side,
//	         expiration, signatureType, timestamp, metadata, builder, signature }
//	owner, orderType, deferExec, postOnly
func TestV2OrderBodyShape(t *testing.T) {
	req := orderRequestV2{
		Order: orderedOrderV2{
			Salt:          1,
			Maker:         "0xMK",
			Signer:        "0xSG",
			TokenId:       "T",
			MakerAmount:   "100",
			TakerAmount:   "200",
			Side:          "BUY",
			Expiration:    "0",
			SignatureType: 3,
			Timestamp:     "1700000000000",
			Metadata:      "0xM",
			Builder:       "0xB",
			Signature:     "0xSIG",
		},
		Owner:     "owner-key",
		OrderType: "GTC",
		DeferExec: false,
		PostOnly:  false,
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	want := `{"order":{"salt":1,"maker":"0xMK","signer":"0xSG","tokenId":"T","makerAmount":"100","takerAmount":"200","side":"BUY","expiration":"0","signatureType":3,"timestamp":"1700000000000","metadata":"0xM","builder":"0xB","signature":"0xSIG"},"owner":"owner-key","orderType":"GTC","deferExec":false,"postOnly":false}`

	if string(out) != want {
		t.Fatalf("V2 body shape mismatch:\n  want = %s\n  got  = %s", want, string(out))
	}

	// 关键字段必须出现（防止某次重构把可选字段误删）
	for _, must := range []string{`"expiration":`, `"deferExec":`, `"postOnly":`} {
		if !strings.Contains(string(out), must) {
			t.Errorf("missing required field %s in body", must)
		}
	}
}

// TestV2OrderBodyExpirationGTD 验证非零 Expiration 透传到 wire body。
func TestV2OrderBodyExpirationGTD(t *testing.T) {
	req := orderRequestV2{
		Order: orderedOrderV2{
			Expiration: "1735689600",
		},
		OrderType: "GTD",
	}
	out, _ := json.Marshal(req)
	if !strings.Contains(string(out), `"expiration":"1735689600"`) {
		t.Fatalf("expiration not propagated: %s", string(out))
	}
}
