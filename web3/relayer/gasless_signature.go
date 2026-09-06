package relayer

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
	"github.com/polymas/go-polymarket-sdk/web3"
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
	return c.signSafeTransactionContext(context.Background(), safeTxn, nonce)
}

func (c *GaslessClient) signSafeTransactionContext(ctx context.Context, safeTxn map[string]interface{}, nonce int) ([]byte, error) {
	// Get Safe contract address (proxy address)
	safeAddr, err := c.getSafeProxyAddressContext(ctx)
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

	txHashBytes, err := c.CallContractWithRetry(ctx, callMsg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call getTransactionHash: %w", err)
	}

	var txHash common.Hash
	if len(txHashBytes) == 0 {
		// Counter-factual Safe：合约还没部署（首次交易由 relayer 的 execTransaction
		// 顺带部署），eth_call 对空地址返回空 bytes 而不是 revert，unpack 会失败。
		// 退化到本地按 EIP-712 算 Safe v1.3.0 的 SafeTx 哈希，公式与已部署 Safe 的
		// getTransactionHash 完全一致（已用一个已部署的 Polymarket Safe 交叉验证过，
		// 任意 to/value/data/nonce 下 on-chain 结果与本地计算逐字节相同）。
		txHash = computeSafeTxHashOffChain(safeAddrCommon, c.GetChainID(), to, value, data, operation,
			safeTxGas, baseGas, gasPrice, gasToken, refundReceiver, nonceBig)
	} else {
		// Unpack the result (returns bytes32)
		if err := safeABI.UnpackIntoInterface(&txHash, "getTransactionHash", txHashBytes); err != nil {
			return nil, fmt.Errorf("failed to unpack getTransactionHash result: %w", err)
		}
	}

	// Sign the transaction hash using encode_defunct (same as Python)
	// encode_defunct(hexstr) creates: "\x19Ethereum Signed Message:\n" + len(bytes) + bytes
	messagePrefix := []byte("\x19Ethereum Signed Message:\n")
	messageLenStr := []byte(strconv.Itoa(32)) // txHash is 32 bytes
	messageBytes := append(messagePrefix, messageLenStr...)
	messageBytes = append(messageBytes, txHash.Bytes()...)

	// Hash the message (sign_message does keccak256)
	hash := crypto.Keccak256Hash(messageBytes)
	signature, err := c.GetSigner().SignWithRecovery(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return signature, nil
}

// computeSafeTxHashOffChain 复现 Gnosis Safe v1.3.0 的 getTransactionHash，供合约尚未
// 部署（counter-factual）时本地签名用。EIP-712：
//
//	domainSeparator = keccak256(EIP712Domain(uint256 chainId,address verifyingContract) ‖ chainId ‖ safe)
//	safeTxHash      = keccak256(SafeTx(...) typehash ‖ 各字段)
//	txHash          = keccak256(0x19 0x01 ‖ domainSeparator ‖ safeTxHash)
func computeSafeTxHashOffChain(
	safe common.Address,
	chainID types.ChainID,
	to common.Address,
	value *big.Int,
	data []byte,
	operation uint8,
	safeTxGas, baseGas, gasPrice *big.Int,
	gasToken, refundReceiver common.Address,
	nonce *big.Int,
) common.Hash {
	domainTypeHash := crypto.Keccak256([]byte("EIP712Domain(uint256 chainId,address verifyingContract)"))
	domainSeparator := crypto.Keccak256(
		domainTypeHash,
		common.LeftPadBytes(big.NewInt(int64(chainID)).Bytes(), 32),
		common.LeftPadBytes(safe.Bytes(), 32),
	)

	safeTxTypeHash := crypto.Keccak256([]byte(
		"SafeTx(address to,uint256 value,bytes data,uint8 operation,uint256 safeTxGas,uint256 baseGas,uint256 gasPrice,address gasToken,address refundReceiver,uint256 nonce)",
	))
	dataHash := crypto.Keccak256(data)
	opBytes := make([]byte, 32)
	opBytes[31] = operation

	safeTxHash := crypto.Keccak256(
		safeTxTypeHash,
		common.LeftPadBytes(to.Bytes(), 32),
		common.LeftPadBytes(value.Bytes(), 32),
		dataHash,
		opBytes,
		common.LeftPadBytes(safeTxGas.Bytes(), 32),
		common.LeftPadBytes(baseGas.Bytes(), 32),
		common.LeftPadBytes(gasPrice.Bytes(), 32),
		common.LeftPadBytes(gasToken.Bytes(), 32),
		common.LeftPadBytes(refundReceiver.Bytes(), 32),
		common.LeftPadBytes(nonce.Bytes(), 32),
	)

	return common.BytesToHash(crypto.Keccak256([]byte{0x19, 0x01}, domainSeparator, safeTxHash))
}

// getSafeProxyAddress gets the Safe proxy address for the base address
func (c *GaslessClient) getSafeProxyAddress() (types.EthAddress, error) {
	return c.getSafeProxyAddressContext(context.Background())
}

func (c *GaslessClient) getSafeProxyAddressContext(ctx context.Context) (types.EthAddress, error) {
	// Safe proxy factory address
	safeProxyFactoryAddr := common.HexToAddress(internal.SafeProxyFactory)

	// Parse SafeProxyFactory ABI
	safeProxyFactoryABI, err := web3.GetSafeProxyFactoryABI()
	if err != nil {
		return "", fmt.Errorf("failed to parse safe proxy factory ABI: %w", err)
	}

	// Call computeProxyAddress(address owner) on SafeProxyFactory
	addr := common.HexToAddress(string(c.GetBaseAddress()))
	packed, err := safeProxyFactoryABI.Pack("computeProxyAddress", addr)
	if err != nil {
		return "", fmt.Errorf("failed to pack computeProxyAddress: %w", err)
	}

	// Call the contract
	callMsg := ethereum.CallMsg{
		To:   &safeProxyFactoryAddr,
		Data: packed,
	}

	result, err := c.CallContractWithRetry(ctx, callMsg, nil)
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

// getSafeNonceOnChain reads the current nonce from the Safe proxy contract on-chain.
// This is the ground-truth nonce — the Safe contract's storage slot that execTransaction
// checks against. Relayer's /nonce endpoint is a cache and can diverge from chain state.
func (c *GaslessClient) getSafeNonceOnChain(ctx context.Context) (uint64, error) {
	safeAddr, err := c.getSafeProxyAddressContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get safe address: %w", err)
	}

	safeABI, err := getSafeABI()
	if err != nil {
		return 0, fmt.Errorf("failed to parse safe ABI: %w", err)
	}

	packed, err := safeABI.Pack("nonce")
	if err != nil {
		return 0, fmt.Errorf("failed to pack nonce call: %w", err)
	}

	safeAddrCommon := common.HexToAddress(string(safeAddr))
	callMsg := ethereum.CallMsg{
		To:   &safeAddrCommon,
		Data: packed,
	}

	result, err := c.CallContractWithRetry(ctx, callMsg, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to call safe.nonce(): %w", err)
	}

	// Counter-factual Safe：地址通过 CREATE2 推导出来但合约还没部署，
	// eth_call 此时对一个 EOA-like 地址返回空 bytes（不是 revert）。
	// Relayer 在首次 execTransaction 时会先把 Safe 部署起来，初始 nonce=0
	// —— 因此 on-chain 空响应等价于 nonce=0，与 relayer /nonce 返回值一致。
	if len(result) == 0 {
		return 0, nil
	}

	var nonceBig *big.Int
	err = safeABI.UnpackIntoInterface(&nonceBig, "nonce", result)
	if err != nil {
		return 0, fmt.Errorf("failed to unpack nonce result: %w", err)
	}

	return nonceBig.Uint64(), nil
}

// getSafeABI returns a minimal ABI for the Safe contract
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
		},
		{
			"inputs": [],
			"name": "nonce",
			"outputs": [
				{"internalType": "uint256", "name": "", "type": "uint256"}
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
