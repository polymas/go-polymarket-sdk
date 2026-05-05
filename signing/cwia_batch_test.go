package signing

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// TestCWIABatchHashAgainstOnChainSample 用真实链上样例验证 EIP-712 实现。
//
// 样例：tx 0xf9661f53bfa269f2c26a90622dfbce7cbacee245342caae1d13fc4cab16ad825
// （Polygon mainnet）调用 DepositWalletFactory.proxy([batch], [signature])，
// 其中 batch 是 wallet=0xa497…7891 的一次 pUSD 转账，signature 由 wallet
// owner=0x0322…8e41 用 EIP-712 签名。
//
// 通过 ecrecover 还原签名者并与 wallet.owner() 对比，可证明：
//
//  1. CALL_TYPEHASH / BATCH_TYPEHASH 一致；
//  2. abi.encode 顺序、calls 数组哈希方式一致；
//  3. EIP-712 domain（"DepositWallet" v"1"）一致；
//  4. \x19\x01 摘要计算一致。
//
// 任何字节级偏差都会让 recover 出来的地址不等于 owner。
func TestCWIABatchHashAgainstOnChainSample(t *testing.T) {
	const (
		walletHex   = "0xa497f6d9b942f3a73e24f9642316f0979c677891"
		ownerHex    = "0x0322F0727f134fc6d0C330FeD0436af1aEe18e41"
		nonce       = int64(877)
		deadline    = int64(1777987422)
		callTarget  = "0xc011a7e12a19f7b1f670d46f03b03f3342e82dfb"
		callDataHex = "a9059cbb00000000000000000000000088422c0f9a1e01db293a92f3d5684d6e29a0ba93000000000000000000000000000000000000000000000000000000000001de1b"
		sigHex      = "b17775663e1ff56d99d60214987793d8626b44edeacef39411c47b4e6a7dcc487062a324214a1828a73d03b26a22a27aebdc4a4f7926b746a2dd34ed10b554341c"
	)

	data, _ := hex.DecodeString(callDataHex)
	batch := CWIABatch{
		Wallet:   common.HexToAddress(walletHex),
		Nonce:    big.NewInt(nonce),
		Deadline: big.NewInt(deadline),
		Calls: []CWIACall{{
			Target: common.HexToAddress(callTarget),
			Value:  big.NewInt(0),
			Data:   data,
		}},
	}

	digest := CWIABatchDigest(big.NewInt(137), batch)

	sig, _ := hex.DecodeString(sigHex)
	if len(sig) != 65 {
		t.Fatalf("sig must be 65 bytes, got %d", len(sig))
	}
	// crypto.SigToPub 期望 v ∈ {0,1}，链上签名是 {27,28}
	sig65 := make([]byte, 65)
	copy(sig65, sig)
	if sig65[64] >= 27 {
		sig65[64] -= 27
	}

	pub, err := crypto.SigToPub(digest.Bytes(), sig65)
	if err != nil {
		t.Fatalf("ecrecover failed: %v", err)
	}
	got := crypto.PubkeyToAddress(*pub).Hex()
	if !strings.EqualFold(got, ownerHex) {
		t.Fatalf("EIP-712 mismatch:\n  digest    = %s\n  expected  = %s\n  recovered = %s",
			digest.Hex(), ownerHex, got)
	}
	t.Logf("digest=%s recovered=%s ✓", digest.Hex(), got)
}

// TestSignCWIABatchRoundtrip 用一次性私钥签名一个 Batch，再用 ecrecover 验回，
// 确认 SignCWIABatch 与 CWIABatchDigest 端到端配套。
func TestSignCWIABatchRoundtrip(t *testing.T) {
	pk, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	expectedSigner := crypto.PubkeyToAddress(pk.PublicKey).Hex()

	batch := CWIABatch{
		Wallet:   common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:    big.NewInt(0),
		Deadline: big.NewInt(2_000_000_000),
		Calls: []CWIACall{{
			Target: common.HexToAddress("0x2222222222222222222222222222222222222222"),
			Value:  big.NewInt(0),
			Data:   []byte{0xde, 0xad, 0xbe, 0xef},
		}},
	}
	sig, err := SignCWIABatch(pk, big.NewInt(137), batch)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 65 || (sig[64] != 27 && sig[64] != 28) {
		t.Fatalf("bad signature shape: len=%d v=%d", len(sig), sig[64])
	}
	digest := CWIABatchDigest(big.NewInt(137), batch)
	sig0 := append([]byte{}, sig...)
	sig0[64] -= 27
	pub, err := crypto.SigToPub(digest.Bytes(), sig0)
	if err != nil {
		t.Fatal(err)
	}
	got := crypto.PubkeyToAddress(*pub).Hex()
	if !strings.EqualFold(got, expectedSigner) {
		t.Fatalf("recovered=%s want=%s", got, expectedSigner)
	}
}
