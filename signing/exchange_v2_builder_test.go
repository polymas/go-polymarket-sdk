package signing

import (
	"bytes"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/types"
)

// Known-answer test for the V2 EIP-712 typehash string. If this ever drifts
// from the doc-specified order, every V2 signature we produce silently breaks.
func TestV2OrderTypehashMatchesDoc(t *testing.T) {
	want := crypto.Keccak256Hash([]byte(
		"Order(uint256 salt,address maker,address signer,uint256 tokenId," +
			"uint256 makerAmount,uint256 takerAmount,uint8 side,uint8 signatureType," +
			"uint256 timestamp,bytes32 metadata,bytes32 builder)",
	))
	if V2OrderTypehash != want {
		t.Fatalf("V2OrderTypehash mismatch\n got: %s\nwant: %s", V2OrderTypehash.Hex(), want.Hex())
	}
}

// Domain separator is deterministic. Baseline — future regressions will be
// caught here; the exact hex can be cross-checked once V2 TS/Py SDK vectors
// are published.
func TestV2DomainSeparatorStable(t *testing.T) {
	chainID := big.NewInt(137)
	exchange := common.HexToAddress("0xE111180000d2663C0091e4f400237545B87B996B")

	got, err := buildV2DomainSeparator(chainID, exchange)
	if err != nil {
		t.Fatalf("buildV2DomainSeparator: %v", err)
	}

	// Repeat — should always match itself.
	got2, err := buildV2DomainSeparator(chainID, exchange)
	if err != nil {
		t.Fatalf("buildV2DomainSeparator (repeat): %v", err)
	}
	if got != got2 {
		t.Fatalf("domain separator not deterministic: %s vs %s", got.Hex(), got2.Hex())
	}
	if got == (common.Hash{}) {
		t.Fatal("domain separator is zero")
	}
	t.Logf("V2 domain separator (chain=137, exchange=0xE111...): %s", got.Hex())
}

// End-to-end: a signed order's signature must ecrecover back to the signer's address.
func TestBuildSignedV2OrderRecoverable(t *testing.T) {
	priv, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	maker := crypto.PubkeyToAddress(priv.PublicKey).Hex()

	data := &V2OrderData{
		Maker:         maker,
		TokenID:       "71321045679252212594626385532706912750332728571942532289631379312455583992563",
		MakerAmount:   "5000000",  // 5 USDC
		TakerAmount:   "10000000", // 10 shares
		Side:          V2SideBUY,
		SignatureType: types.EOASignatureType,
		TimestampMS:   1_714_305_600_000, // fixed for deterministic digest
		Salt:          big.NewInt(42),
	}
	chainID := big.NewInt(137)
	exchange := common.HexToAddress("0xE111180000d2663C0091e4f400237545B87B996B")

	signed, err := BuildSignedV2Order(priv, data, chainID, exchange)
	if err != nil {
		t.Fatalf("BuildSignedV2Order: %v", err)
	}
	if len(signed.Signature) != 65 {
		t.Fatalf("signature length: got %d, want 65", len(signed.Signature))
	}
	if v := signed.Signature[64]; v != 27 && v != 28 {
		t.Fatalf("signature v byte: got %d, want 27 or 28", v)
	}

	digest, err := V2OrderDigest(&signed.V2Order, chainID, exchange)
	if err != nil {
		t.Fatalf("digest: %v", err)
	}

	// Ecrecover expects v ∈ {0,1}; undo the +27.
	sig := make([]byte, 65)
	copy(sig, signed.Signature)
	sig[64] -= 27

	recoveredPubBytes, err := crypto.Ecrecover(digest.Bytes(), sig)
	if err != nil {
		t.Fatalf("ecrecover: %v", err)
	}
	recoveredPub, err := crypto.UnmarshalPubkey(recoveredPubBytes)
	if err != nil {
		t.Fatalf("unmarshal pubkey: %v", err)
	}
	recoveredAddr := crypto.PubkeyToAddress(*recoveredPub)
	expectedAddr := crypto.PubkeyToAddress(priv.PublicKey)
	if !bytes.Equal(recoveredAddr.Bytes(), expectedAddr.Bytes()) {
		t.Fatalf("recovered addr mismatch\n got: %s\nwant: %s", recoveredAddr.Hex(), expectedAddr.Hex())
	}
}

// Deterministic digest: same inputs → same digest. Guards against accidental
// source-of-entropy creep into the signing path.
func TestV2OrderDigestDeterministic(t *testing.T) {
	priv, _ := crypto.GenerateKey()
	data := sampleV2OrderData(priv)

	order1, err := buildV2Order(data)
	if err != nil {
		t.Fatalf("buildV2Order: %v", err)
	}
	order2, err := buildV2Order(data)
	if err != nil {
		t.Fatalf("buildV2Order (repeat): %v", err)
	}

	chainID := big.NewInt(137)
	exchange := common.HexToAddress("0xE111180000d2663C0091e4f400237545B87B996B")

	d1, err := V2OrderDigest(order1, chainID, exchange)
	if err != nil {
		t.Fatalf("digest 1: %v", err)
	}
	d2, err := V2OrderDigest(order2, chainID, exchange)
	if err != nil {
		t.Fatalf("digest 2: %v", err)
	}
	if d1 != d2 {
		t.Fatalf("digest non-deterministic: %s vs %s", d1.Hex(), d2.Hex())
	}
}

func sampleV2OrderData(priv *ecdsa.PrivateKey) *V2OrderData {
	return &V2OrderData{
		Maker:         crypto.PubkeyToAddress(priv.PublicKey).Hex(),
		TokenID:       "1",
		MakerAmount:   "1000000",
		TakerAmount:   "2000000",
		Side:          V2SideBUY,
		SignatureType: types.EOASignatureType,
		TimestampMS:   1_714_305_600_000,
		Salt:          big.NewInt(1),
	}
}
