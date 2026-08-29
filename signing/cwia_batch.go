package signing

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Polymarket DepositWallet (ERC-7760) EIP-712 batch signing.
//
// 链上验证位于 DepositWallet.execute(Batch, bytes)：
//   bytes32 batchHash   = WalletLib.hash(batch)
//   bytes32 batchDigest = _hashTypedData(batchHash)        // EIP-712 digest
//   require(isValidSignature(batchDigest, signature) == 0x1626ba7e)
//
// 域：name="DepositWallet", version="1", chainId=137, verifyingContract=<wallet>
//
// 已对照真实链上 tx 0xf966…d825 验证：ecrecover 还原签名者 ==
// wallet.owner()，证明本实现与 solady ERC1271 + WalletLib 完全一致。

// CWIACall 是 Batch 中的单条调用（与 Solidity 端 Call 结构对应）。
type CWIACall struct {
	Target common.Address
	Value  *big.Int // 通常为 0
	Data   []byte
}

// CWIABatch 表示一次原子批量执行的全部参数。
type CWIABatch struct {
	Wallet   common.Address // DepositWallet 代理地址
	Nonce    *big.Int       // 必须等于 wallet.nonce() 的当前值
	Deadline *big.Int       // 链上 block.timestamp 必须 <= 此值
	Calls    []CWIACall
}

// 由 WalletLib.sol 中常量复刻：
//
//	CALL_TYPEHASH = keccak256("Call(address target,uint256 value,bytes data)")
//	BATCH_TYPESTR = "Batch(address wallet,uint256 nonce,uint256 deadline,Call[] calls)Call(...)"
//	BATCH_TYPEHASH = keccak256(BATCH_TYPESTR)
var (
	cwiaCallTypeHash   = crypto.Keccak256Hash([]byte("Call(address target,uint256 value,bytes data)"))
	cwiaBatchTypeStr   = []byte("Batch(address wallet,uint256 nonce,uint256 deadline,Call[] calls)Call(address target,uint256 value,bytes data)")
	cwiaBatchTypeHash  = crypto.Keccak256Hash(cwiaBatchTypeStr)
	cwiaDomainTypeHash = crypto.Keccak256Hash([]byte(
		"EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"))
	cwiaDomainName    = crypto.Keccak256Hash([]byte("DepositWallet"))
	cwiaDomainVersion = crypto.Keccak256Hash([]byte("1"))
)

// HashCWIACall 复刻 WalletLib.hash(Call)：
//
//	keccak256(abi.encode(CALL_TYPEHASH, target, value, keccak256(data)))
func HashCWIACall(c CWIACall) common.Hash {
	dataHash := crypto.Keccak256Hash(c.Data)
	enc := encodeAbi(
		"bytes32", cwiaCallTypeHash,
		"address", c.Target,
		"uint256", c.Value,
		"bytes32", dataHash,
	)
	return crypto.Keccak256Hash(enc)
}

// HashCWIACalls 复刻 WalletLib.hash(Call[])：
//
//	keccak256(concat(hash(call_i)))    // 注意：abi.encodePacked，非 abi.encode
func HashCWIACalls(calls []CWIACall) common.Hash {
	buf := make([]byte, 0, len(calls)*32)
	for _, c := range calls {
		h := HashCWIACall(c)
		buf = append(buf, h.Bytes()...)
	}
	return crypto.Keccak256Hash(buf)
}

// HashCWIABatch 复刻 WalletLib.hash(Batch)：
//
//	keccak256(abi.encode(BATCH_TYPEHASH, wallet, nonce, deadline, hash(calls)))
func HashCWIABatch(b CWIABatch) common.Hash {
	callsHash := HashCWIACalls(b.Calls)
	enc := encodeAbi(
		"bytes32", cwiaBatchTypeHash,
		"address", b.Wallet,
		"uint256", b.Nonce,
		"uint256", b.Deadline,
		"bytes32", callsHash,
	)
	return crypto.Keccak256Hash(enc)
}

// CWIADomainSeparator 计算 DepositWallet 的 EIP-712 domain separator。
//
//	keccak256(abi.encode(
//	    keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
//	    keccak256("DepositWallet"),
//	    keccak256("1"),
//	    chainId,
//	    wallet
//	))
func CWIADomainSeparator(chainID *big.Int, wallet common.Address) common.Hash {
	enc := encodeAbi(
		"bytes32", cwiaDomainTypeHash,
		"bytes32", cwiaDomainName,
		"bytes32", cwiaDomainVersion,
		"uint256", chainID,
		"address", wallet,
	)
	return crypto.Keccak256Hash(enc)
}

// CWIABatchDigest 计算签名摘要：keccak256("\x19\x01" || domainSep || batchHash)。
func CWIABatchDigest(chainID *big.Int, b CWIABatch) common.Hash {
	domain := CWIADomainSeparator(chainID, b.Wallet)
	batchHash := HashCWIABatch(b)
	buf := make([]byte, 0, 2+32+32)
	buf = append(buf, 0x19, 0x01)
	buf = append(buf, domain.Bytes()...)
	buf = append(buf, batchHash.Bytes()...)
	return crypto.Keccak256Hash(buf)
}

// SignCWIABatch 用私钥签名 Batch，返回 65 字节 r||s||v 签名（v ∈ {27,28}）。
//
// solady 的 ERC1271 在 isValidSignature 内部 ecrecover 拿出 EOA 后会检查
// `== owner()`，所以签名者必须是 wallet.owner()。
//
// Deprecated: 新代码请使用 SignCWIABatchWithSigner，避免传递裸私钥。
func SignCWIABatch(privKey *ecdsa.PrivateKey, chainID *big.Int, b CWIABatch) ([]byte, error) {
	return SignCWIABatchWithSigner(privateKeyRecoverySigner{privateKey: privKey}, chainID, b)
}

// SignCWIABatchWithSigner 使用受限签名器签署 Deposit Wallet Batch。
func SignCWIABatchWithSigner(signer RecoverySigner, chainID *big.Int, b CWIABatch) ([]byte, error) {
	if signer == nil {
		return nil, fmt.Errorf("batch signer is nil")
	}
	if len(b.Calls) == 0 {
		return nil, fmt.Errorf("batch must contain at least one call")
	}
	digest := CWIABatchDigest(chainID, b)
	sig, err := signer.SignWithRecovery(digest)
	if err != nil {
		return nil, fmt.Errorf("ecdsa sign failed: %w", err)
	}
	if err := validateRecoverySignature(sig); err != nil {
		return nil, fmt.Errorf("ecdsa sign failed: %w", err)
	}
	return sig, nil
}

// encodeAbi 是简化的 abi.encode 包装：传入 (typeStr, value, ...) 重复对。
// 仅支持本文件用到的 bytes32/address/uint256 类型。
func encodeAbi(pairs ...interface{}) []byte {
	if len(pairs)%2 != 0 {
		panic("encodeAbi: pairs must come in (type,value) groups")
	}
	args := make(abi.Arguments, 0, len(pairs)/2)
	values := make([]interface{}, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		typStr := pairs[i].(string)
		t, err := abi.NewType(typStr, "", nil)
		if err != nil {
			panic(fmt.Sprintf("encodeAbi: bad type %q: %v", typStr, err))
		}
		args = append(args, abi.Argument{Type: t})
		v := pairs[i+1]
		// 把 common.Hash 转 [32]byte 以适配 bytes32
		if h, ok := v.(common.Hash); ok {
			var arr [32]byte
			copy(arr[:], h.Bytes())
			v = arr
		}
		values = append(values, v)
	}
	out, err := args.Pack(values...)
	if err != nil {
		panic(fmt.Sprintf("encodeAbi: pack failed: %v", err))
	}
	return out
}
