package web3

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// SignatureParams 表示中继交易的签名参数
// 字段顺序与Python版本匹配以确保JSON键顺序一致
type SignatureParams struct {
	GasPrice   string `json:"gasPrice"`
	GasLimit   string `json:"gasLimit"`
	RelayerFee string `json:"relayerFee"`
	RelayHub   string `json:"relayHub"`
	Relay      string `json:"relay"`
}

// ProxyRelayBody 表示代理中继交易的请求体
// 字段顺序与Python版本匹配以确保HMAC签名的JSON键顺序一致
type ProxyRelayBody struct {
	Data            string          `json:"data"`
	From            string          `json:"from"`
	Metadata        string          `json:"metadata"`
	Nonce           string          `json:"nonce"`
	ProxyWallet     string          `json:"proxyWallet"`
	Signature       string          `json:"signature"`
	SignatureParams SignatureParams `json:"signatureParams"`
	To              string          `json:"to"`
	Type            string          `json:"type"`
}

// SafeSignatureParams 表示Safe中继交易的签名参数
// 字段顺序与Python版本匹配以确保JSON键顺序一致
type SafeSignatureParams struct {
	BaseGas        string `json:"baseGas"`
	GasPrice       string `json:"gasPrice"`
	GasToken       string `json:"gasToken"`
	Operation      string `json:"operation"`
	RefundReceiver string `json:"refundReceiver"`
	SafeTxnGas     string `json:"safeTxnGas"`
}

// SafeRelayBody 表示Safe中继交易的请求体
// 字段顺序与Python版本匹配以确保HMAC签名的JSON键顺序一致
type SafeRelayBody struct {
	Data            string              `json:"data"`
	From            string              `json:"from"`
	Metadata        string              `json:"metadata"`
	Nonce           string              `json:"nonce"`
	ProxyWallet     string              `json:"proxyWallet"`
	Signature       string              `json:"signature"`
	SignatureParams SafeSignatureParams `json:"signatureParams"`
	To              string              `json:"to"`
	Type            string              `json:"type"`
}

// signSafeTransaction signs a Safe transaction
func (c *GaslessClient) signSafeTransaction(
	safeTxn map[string]interface{},
	nonce int,
) ([]byte, error) {
	// Get Safe contract address (proxy address)
	safeAddr, err := c.getSafeProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get safe address: %w", err)
	}

	// Parse Safe ABI
	safeABI, err := getSafeABI()
	if err != nil {
		return nil, fmt.Errorf("failed to parse safe ABI: %w", err)
	}

	// Call getTransactionHash on Safe contract
	// getTransactionHash(
	//   address to,
	//   uint256 value,
	//   bytes data,
	//   uint8 operation,
	//   uint256 safeTxGas,
	//   uint256 baseGas,
	//   uint256 gasPrice,
	//   address gasToken,
	//   address refundReceiver,
	//   uint256 nonce
	// )
	to := common.HexToAddress(safeTxn["to"].(string))
	value := big.NewInt(int64(safeTxn["value"].(int)))
	dataHex := safeTxn["data"].(string)
	data, err := hex.DecodeString(strings.TrimPrefix(dataHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to decode data: %w", err)
	}
	operation := uint8(safeTxn["operation"].(int))
	safeTxGas := big.NewInt(0)
	baseGas := big.NewInt(0)
	gasPrice := big.NewInt(0)
	gasToken := common.HexToAddress(internal.ZeroAddress)
	refundReceiver := common.HexToAddress(internal.ZeroAddress)
	nonceBig := big.NewInt(int64(nonce))

	// Pack the function call
	packed, err := safeABI.Pack("getTransactionHash",
		to,
		value,
		data,
		operation,
		safeTxGas,
		baseGas,
		gasPrice,
		gasToken,
		refundReceiver,
		nonceBig,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to pack getTransactionHash: %w", err)
	}

	// Call the contract
	safeAddrCommon := common.HexToAddress(string(safeAddr))
	callMsg := ethereum.CallMsg{
		To:   &safeAddrCommon,
		Data: packed,
	}

	txHashBytes, err := c.callContractWithRetry(context.Background(), callMsg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call getTransactionHash: %w", err)
	}

	// Unpack the result (returns bytes32)
	var txHash common.Hash
	err = safeABI.UnpackIntoInterface(&txHash, "getTransactionHash", txHashBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack getTransactionHash result: %w", err)
	}

	// Sign the transaction hash using encode_defunct (same as Python)
	// encode_defunct(hexstr) creates: "\x19Ethereum Signed Message:\n" + len(bytes) + bytes
	messagePrefix := []byte("\x19Ethereum Signed Message:\n")
	messageLenStr := []byte(strconv.Itoa(32)) // txHash is 32 bytes
	messageBytes := append(messagePrefix, messageLenStr...)
	messageBytes = append(messageBytes, txHash.Bytes()...)

	// Hash the message (sign_message does keccak256)
	hash := crypto.Keccak256Hash(messageBytes)
	signature, err := c.signer.SignWithRecovery(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return signature, nil
}

// getSafeProxyAddress gets the Safe proxy address for the base address
func (c *GaslessClient) getSafeProxyAddress() (types.EthAddress, error) {
	// Safe proxy factory address
	safeProxyFactoryAddr := common.HexToAddress(internal.SafeProxyFactory)

	// Parse SafeProxyFactory ABI
	safeProxyFactoryABI, err := getSafeProxyFactoryABI()
	if err != nil {
		return "", fmt.Errorf("failed to parse safe proxy factory ABI: %w", err)
	}

	// Call computeProxyAddress(address owner) on SafeProxyFactory
	addr := common.HexToAddress(string(c.baseAddress))
	packed, err := safeProxyFactoryABI.Pack("computeProxyAddress", addr)
	if err != nil {
		return "", fmt.Errorf("failed to pack computeProxyAddress: %w", err)
	}

	// Call the contract
	callMsg := ethereum.CallMsg{
		To:   &safeProxyFactoryAddr,
		Data: packed,
	}

	result, err := c.callContractWithRetry(context.Background(), callMsg, nil)
	if err != nil {
		return "", fmt.Errorf("failed to call computeProxyAddress: %w", err)
	}

	// Unpack the result - computeProxyAddress returns (address)
	var proxyAddr common.Address
	err = safeProxyFactoryABI.UnpackIntoInterface(&proxyAddr, "computeProxyAddress", result)
	if err != nil {
		return "", fmt.Errorf("failed to unpack result: %w", err)
	}

	return types.EthAddress(proxyAddr.Hex()), nil
}

// getSafeABI returns a minimal ABI for the Safe contract
// Only includes getTransactionHash function
func getSafeABI() (*abi.ABI, error) {
	abiJSON := `[
		{
			"inputs": [
				{"internalType": "address", "name": "to", "type": "address"},
				{"internalType": "uint256", "name": "value", "type": "uint256"},
				{"internalType": "bytes", "name": "data", "type": "bytes"},
				{"internalType": "uint8", "name": "operation", "type": "uint8"},
				{"internalType": "uint256", "name": "safeTxGas", "type": "uint256"},
				{"internalType": "uint256", "name": "baseGas", "type": "uint256"},
				{"internalType": "uint256", "name": "gasPrice", "type": "uint256"},
				{"internalType": "address", "name": "gasToken", "type": "address"},
				{"internalType": "address", "name": "refundReceiver", "type": "address"},
				{"internalType": "uint256", "name": "_nonce", "type": "uint256"}
			],
			"name": "getTransactionHash",
			"outputs": [
				{"internalType": "bytes32", "name": "", "type": "bytes32"}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`

	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}

	return &parsedABI, nil
}
