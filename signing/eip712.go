package signing

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	ClobDomainName = "ClobAuthDomain"
	ClobVersion    = "1"
	MsgToSign      = "This message attests that I control the given wallet"
)

// SignClobAuthMessage 对CLOB认证消息进行签名
func SignClobAuthMessage(signer *Signer, timestamp int64, nonce int) (string, error) {
	chainID := signer.ChainID()

	// Create domain separator
	domainHash := getClobAuthDomainHash(chainID)

	// Create message hash
	messageHash := getClobAuthMessageHash(
		signer.Address(),
		timestamp,
		nonce,
	)

	// Combine domain and message
	combined := append([]byte("\x19\x01"), domainHash.Bytes()...)
	combined = append(combined, messageHash.Bytes()...)

	// Hash the combined data
	hash := crypto.Keccak256Hash(combined)

	// Sign the hash
	// Note: For EIP-712, we need the full 65-byte signature (including recovery ID)
	// Python's signer.sign() returns the full signature with recovery ID
	sigWithRecovery, err := signer.SignWithRecovery(hash)
	if err != nil {
		return "", err
	}

	// Return as hex string with 0x prefix (including recovery ID)
	return "0x" + common.Bytes2Hex(sigWithRecovery), nil
}

// getClobAuthDomainHash creates the domain separator hash
func getClobAuthDomainHash(chainID types.ChainID) common.Hash {
	// EIP-712 domain separator
	domainTypeHash := crypto.Keccak256Hash([]byte("EIP712Domain(string name,string version,uint256 chainId)"))
	nameHash := crypto.Keccak256Hash([]byte(ClobDomainName))
	versionHash := crypto.Keccak256Hash([]byte(ClobVersion))

	// Encode chain ID as uint256
	chainIDBytes := make([]byte, 32)
	chainIDBytes[31] = byte(chainID)
	if chainID > 255 {
		chainIDBytes[30] = byte(chainID >> 8)
	}

	// Combine domain fields
	domainData := append(domainTypeHash.Bytes(), nameHash.Bytes()...)
	domainData = append(domainData, versionHash.Bytes()...)
	domainData = append(domainData, chainIDBytes...)

	return crypto.Keccak256Hash(domainData)
}

// getClobAuthMessageHash creates the message hash
func getClobAuthMessageHash(address types.EthAddress, timestamp int64, nonce int) common.Hash {
	// ClobAuth type hash
	typeHash := crypto.Keccak256Hash([]byte("ClobAuth(address address,string timestamp,uint256 nonce,string message)"))

	// Address (padded to 32 bytes)
	addrBytes := common.HexToAddress(string(address)).Bytes()
	addrPadded := make([]byte, 32)
	copy(addrPadded[12:], addrBytes)

	// Timestamp hash
	timestampStr := fmt.Sprintf("%d", timestamp)
	timestampHash := crypto.Keccak256Hash([]byte(timestampStr))

	// Nonce (padded to 32 bytes)
	nonceBytes := make([]byte, 32)
	nonceBytes[31] = byte(nonce)
	if nonce > 255 {
		nonceBytes[30] = byte(nonce >> 8)
	}

	// Message hash
	messageHash := crypto.Keccak256Hash([]byte(MsgToSign))

	// Combine message fields
	messageData := append(typeHash.Bytes(), addrPadded...)
	messageData = append(messageData, timestampHash.Bytes()...)
	messageData = append(messageData, nonceBytes...)
	messageData = append(messageData, messageHash.Bytes()...)

	return crypto.Keccak256Hash(messageData)
}
