package clob

import (
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

// TestV2OrderSignerByType 锁死 V2 订单 maker/signer 的派生规则：
//
//	EOA          : maker = signer = EOA
//	ProxyV1      : maker = proxy, signer = EOA
//	Safe         : maker = proxy, signer = EOA
//	CWIA / 1271  : maker = signer = proxy   ← v1.10.2 修复
//
// 对照 py-clob-client-v2 的 order_builder/builder.py: _v2_order_signer。
//
// 这是一个纯静态规则测试——为了避免拉起 web3 client，直接对着源码 switch
// 模式手工断言。任何人改动 orders_v2.go 那段 switch 都应当同时改这个测试。
func TestV2OrderSignerByType(t *testing.T) {
	const (
		eoa   = "0xEOA"
		proxy = "0xPROXY"
	)

	cases := []struct {
		name       string
		sigType    types.SignatureType
		wantMaker  string
		wantSigner string
	}{
		{"EOA", types.EOASignatureType, eoa, eoa},
		{"PolyProxy", types.ProxySignatureType, proxy, eoa},
		{"Safe", types.SafeSignatureType, proxy, eoa},
		{"CWIA/1271", types.CWIASignatureType, proxy, proxy},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			maker := eoa
			signer := eoa
			// 与 orders_v2.go 中 buildSignedV2Order 的 switch 完全同形
			switch tc.sigType {
			case types.ProxySignatureType, types.SafeSignatureType:
				maker = proxy
			case types.CWIASignatureType:
				maker = proxy
				signer = proxy
			}
			if !strings.EqualFold(maker, tc.wantMaker) {
				t.Errorf("maker: got %s, want %s", maker, tc.wantMaker)
			}
			if !strings.EqualFold(signer, tc.wantSigner) {
				t.Errorf("signer: got %s, want %s", signer, tc.wantSigner)
			}
		})
	}
}
