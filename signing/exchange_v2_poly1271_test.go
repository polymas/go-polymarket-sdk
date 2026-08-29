package signing

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/types"
)

// TestSignV2OrderPoly1271AgainstPython 用与官方 py-clob-client-v2
// _build_poly_1271_order_signature 完全相同的输入，验证 Go 实现产出
// 字节级一致的签名。
//
// 对照流程见 cwia 调研笔记；reference signature 由 Python 离线生成，
// 包含 inner_sig (65) || appDomain (32) || contents (32) || ORDER_TYPE_STRING || uint16(len)。
func TestSignV2OrderPoly1271AgainstPython(t *testing.T) {
	const (
		pkHex      = "1111111111111111111111111111111111111111111111111111111111111111"
		exchange   = "0xE111180000d2663C0091e4f400237545B87B996B" // PolygonExchangeV2
		maker      = "0x1111111111111111111111111111111111111111"
		walletAddr = "0x2222222222222222222222222222222222222222"
		// 与 python 参考脚本完全一致的字面量；任何位偏差都会导致 ecrecover
		// 还原出错误的签名者，进而最终签名不一致。
		expected = "0x922e220dea8c59dd0b7580556b0af765cbdc27366a0ec42bcd678c6012802b462d17267484cad8dda548ff2102cbba64a25d9a098941f88b46f15d091dce13c91c3264e159346253e26a64e00b69032db0e7d32f94628de3e6eecb50304d7af3d23d5ecafc34d0897d74e3e08da0671603183d1952db58ff429e22ea2eea91086e4f726465722875696e743235362073616c742c61646472657373206d616b65722c61646472657373207369676e65722c75696e7432353620746f6b656e49642c75696e74323536206d616b6572416d6f756e742c75696e743235362074616b6572416d6f756e742c75696e743820736964652c75696e7438207369676e6174757265547970652c75696e743235362074696d657374616d702c62797465733332206d657461646174612c62797465733332206275696c6465722900ba"
	)

	signer, err := NewSigner(pkHex, types.Polygon)
	if err != nil {
		t.Fatal(err)
	}
	defer signer.Clear()

	order := &V2Order{
		Salt:          big.NewInt(12345),
		Maker:         common.HexToAddress(maker),
		Signer:        common.HexToAddress(walletAddr),
		TokenID:       big.NewInt(9999),
		MakerAmount:   big.NewInt(100_000_000),
		TakerAmount:   big.NewInt(200_000_000),
		Side:          V2SideBUY,
		SignatureType: types.CWIASignatureType,
		TimestampMS:   big.NewInt(1_700_000_000_000),
		// Metadata, Builder 默认零
	}

	sig, err := signV2OrderPoly1271(
		order,
		big.NewInt(137),
		common.HexToAddress(exchange),
		signer,
	)
	if err != nil {
		t.Fatal(err)
	}
	got := "0x" + hex.EncodeToString(sig)
	if got != expected {
		t.Fatalf("POLY_1271 signature mismatch\n  expected = %s\n  got      = %s", expected, got)
	}
	t.Logf("POLY_1271 sig matches Python reference (%d bytes) ✓", len(sig))
}
