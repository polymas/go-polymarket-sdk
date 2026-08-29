package signing

import (
	"crypto/ecdsa"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/types"
)

// RecoverySigner 只暴露摘要签名能力，不允许调用方取得底层私钥。
// 外部签名器、HSM 或策略签名服务可以实现此接口。
type RecoverySigner interface {
	SignWithRecovery(messageHash common.Hash) ([]byte, error)
}

// AddressedSigner 是同时提供签名地址的受限签名器。
type AddressedSigner interface {
	RecoverySigner
	Address() types.EthAddress
}

// ClobSigner 提供 CLOB L1 鉴权所需的最小能力。
type ClobSigner interface {
	AddressedSigner
	ChainID() types.ChainID
}

// Signer 处理签名操作
type Signer struct {
	mu         sync.RWMutex
	privateKey *ecdsa.PrivateKey
	address    types.EthAddress
	chainID    types.ChainID
}

// NewSigner 从私钥创建新的签名器
func NewSigner(privateKeyHex string, chainID types.ChainID) (*Signer, error) {
	// Remove 0x prefix if present
	privateKeyStr := privateKeyHex
	if len(privateKeyStr) >= 2 && privateKeyStr[0:2] == "0x" {
		privateKeyStr = privateKeyStr[2:]
	}

	privateKey, err := crypto.HexToECDSA(privateKeyStr)
	if err != nil {
		return nil, fmt.Errorf("invalid private key format: %w", err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		zeroPrivateKey(privateKey)
		return nil, ErrInvalidPublicKey
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	return &Signer{
		privateKey: privateKey,
		address:    types.EthAddress(address.Hex()),
		chainID:    chainID,
	}, nil
}

// Address 返回以太坊地址
func (s *Signer) Address() types.EthAddress {
	if s == nil {
		return ""
	}
	return s.address
}

// ChainID 返回链ID
func (s *Signer) ChainID() types.ChainID {
	if s == nil {
		return 0
	}
	return s.chainID
}

// Sign 对消息哈希进行签名（返回64字节，不包含恢复ID）
func (s *Signer) Sign(messageHash common.Hash) ([]byte, error) {
	signature, err := s.sign(messageHash)
	if err != nil {
		return nil, err
	}
	// Remove recovery ID (last byte)
	return signature[:len(signature)-1], nil
}

// SignWithRecovery 对消息哈希进行签名并返回包含v值的完整签名（65字节）
// 返回Python兼容格式的签名：v值（27/28）而不是恢复ID（0/1）
func (s *Signer) SignWithRecovery(messageHash common.Hash) ([]byte, error) {
	signature, err := s.sign(messageHash)
	if err != nil {
		return nil, err
	}

	// Python's sign_message returns signature with v value (27/28), not recovery ID (0/1)
	// Go's crypto.Sign returns recovery ID (0/1) in the last byte
	// Convert recovery ID to v value: v = recovery_id + 27 (to match Python format)
	if len(signature) != 65 || signature[64] > 1 {
		return nil, fmt.Errorf("%w: invalid recovery signature", ErrSigningFailed)
	}
	// Convert recovery ID (0/1) to v value (27/28) to match Python format.
	signature[64] += 27

	return signature, nil
}

func (s *Signer) sign(messageHash common.Hash) ([]byte, error) {
	if s == nil {
		return nil, ErrSignerClosed
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.privateKey == nil {
		return nil, ErrSignerClosed
	}
	return crypto.Sign(messageHash.Bytes(), s.privateKey)
}

// SignBytes 对原始字节进行签名（先转换为哈希）
func (s *Signer) SignBytes(data []byte) ([]byte, error) {
	hash := crypto.Keccak256Hash(data)
	return s.Sign(hash)
}

// SignHash 对哈希进行签名并返回十六进制字符串格式的签名
func (s *Signer) SignHash(hash common.Hash) (string, error) {
	sig, err := s.Sign(hash)
	if err != nil {
		return "", err
	}
	return "0x" + common.Bytes2Hex(sig), nil
}

// SignMessage 对以太坊消息进行签名（EIP-191）
func (s *Signer) SignMessage(message []byte) ([]byte, error) {
	hash := common.BytesToHash(accounts.TextHash(message))
	return s.Sign(hash)
}

// Clear 清理敏感信息（私钥）
// 安全建议：在不再需要 Signer 时调用此方法清理内存中的私钥
// 注意：清理后 Signer 将无法再使用，请确保在清理前不再需要签名功能
func (s *Signer) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.privateKey != nil {
		zeroPrivateKey(s.privateKey)
		s.privateKey = nil
	}
}

// Closed 报告签名器是否已被清理。
func (s *Signer) Closed() bool {
	if s == nil {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.privateKey == nil
}

// zeroPrivateKey 尽力覆盖当前 ecdsa.PrivateKey 持有的私钥大整数。
// Go 运行时、调用方字符串和历史复制仍可能保留副本，因此这不是绝对安全擦除。
func zeroPrivateKey(privateKey *ecdsa.PrivateKey) {
	if privateKey == nil || privateKey.D == nil {
		return
	}
	words := privateKey.D.Bits()
	for i := range words {
		words[i] = 0
	}
	privateKey.D.SetInt64(0)
}
