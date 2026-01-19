package signing

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymarket/go-polymarket-sdk/types"
)

// Signer 处理签名操作
type Signer struct {
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
	return s.address
}

// ChainID 返回链ID
func (s *Signer) ChainID() types.ChainID {
	return s.chainID
}

// Sign 对消息哈希进行签名（返回64字节，不包含恢复ID）
func (s *Signer) Sign(messageHash common.Hash) ([]byte, error) {
	signature, err := crypto.Sign(messageHash.Bytes(), s.privateKey)
	if err != nil {
		return nil, err
	}
	// Remove recovery ID (last byte)
	return signature[:len(signature)-1], nil
}

// SignWithRecovery 对消息哈希进行签名并返回包含v值的完整签名（65字节）
// 返回Python兼容格式的签名：v值（27-30）而不是恢复ID（0-3）
func (s *Signer) SignWithRecovery(messageHash common.Hash) ([]byte, error) {
	signature, err := crypto.Sign(messageHash.Bytes(), s.privateKey)
	if err != nil {
		return nil, err
	}

	// Python's sign_message returns signature with v value (27-30), not recovery ID (0-3)
	// Go's crypto.Sign returns recovery ID (0-3) in the last byte
	// Convert recovery ID to v value: v = recovery_id + 27 (to match Python format)
	if len(signature) == 65 {
		recoveryID := signature[64]
		if recoveryID < 4 {
			// Convert recovery ID (0-3) to v value (27-30) to match Python format
			signature[64] = recoveryID + 27
		}
		// If already in v value format (27-30), keep as is
	}

	return signature, nil
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

// PrivateKey 返回私钥（用于订单构建器）
func (s *Signer) PrivateKey() *ecdsa.PrivateKey {
	return s.privateKey
}

// Clear 清理敏感信息（私钥）
// 安全建议：在不再需要 Signer 时调用此方法清理内存中的私钥
// 注意：清理后 Signer 将无法再使用，请确保在清理前不再需要签名功能
func (s *Signer) Clear() {
	if s.privateKey != nil {
		// 清零私钥的 D 值（私钥的核心部分）
		if s.privateKey.D != nil {
			zeroBigInt(s.privateKey.D)
		}
		// 将私钥指针设为 nil
		s.privateKey = nil
	}
}

// zeroBigInt 清零 big.Int 的值
func zeroBigInt(bi *big.Int) {
	if bi == nil {
		return
	}
	// 获取字节表示并清零
	bytes := bi.Bytes()
	for i := range bytes {
		bytes[i] = 0
	}
	// 设置 big.Int 为零值
	bi.SetInt64(0)
}
