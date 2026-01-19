package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymarket/go-polymarket-sdk/internal"
	"github.com/polymarket/go-polymarket-sdk/types"
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

// GaslessWeb3Client 是通过中继进行免gas交易的Web3客户端
type GaslessWeb3Client struct {
	*baseWeb3Client            // 嵌入实现类型以访问私有字段
	web3Client      Web3Client // 保存接口引用供外部使用
	httpClient      *http.Client
	relayURL        string
	relayHub        string
	relayAddress    string
	builderCreds    *types.ApiCreds
	localSigner     *LocalSigner
	conditionalABI  *abi.ABI
	negRiskABI      *abi.ABI
	proxyFactoryABI *abi.ABI
	// relayer 调用统计
	relayerCallCount int64 // 使用 atomic 操作，记录总调用次数
}

// NewGaslessWeb3Client creates a new gasless Web3 client
func NewGaslessWeb3Client(
	privateKey string,
	signatureType types.SignatureType,
	chainID types.ChainID,
	builderCreds *types.ApiCreds,
) (*GaslessWeb3Client, error) {
	// Only support proxy (1) and safe (2) wallets
	if signatureType != types.ProxySignatureType && signatureType != types.SafeSignatureType {
		return nil, fmt.Errorf("gaslessWeb3Client only supports signature_type=1 (proxy) and signature_type=2 (safe)")
	}

	baseClient, err := NewBaseWeb3Client(privateKey, signatureType, chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create base client: %w", err)
	}

	// Parse ABIs
	conditionalABI, err := getConditionalTokensABI()
	if err != nil {
		return nil, fmt.Errorf("failed to parse conditional tokens ABI: %w", err)
	}

	negRiskABI, err := getNegRiskAdapterABI()
	if err != nil {
		return nil, fmt.Errorf("failed to parse neg risk adapter ABI: %w", err)
	}

	proxyFactoryABI, err := getProxyFactoryABI()
	if err != nil {
		return nil, fmt.Errorf("failed to parse proxy factory ABI: %w", err)
	}

	// Create LocalSigner for signing requests (matching Python's LocalSigner)
	// 需要类型断言访问私有字段
	baseClientImpl, ok := baseClient.(*baseWeb3Client)
	if !ok {
		return nil, fmt.Errorf("failed to cast base client to implementation type")
	}
	localSigner := NewLocalSigner(baseClientImpl.signer, builderCreds)

	client := &GaslessWeb3Client{
		baseWeb3Client: baseClientImpl,
		web3Client:     baseClient, // 保存接口引用
		httpClient: &http.Client{
			Timeout: internal.HTTPClientLongTimeout,
			// Use default transport (automatically handles proxy, TLS, etc.)
		},
		relayURL:        internal.RelayerDomain,
		relayHub:        internal.RelayHub,
		relayAddress:    internal.RelayAddress,
		builderCreds:    builderCreds,
		localSigner:     localSigner,
		conditionalABI:  conditionalABI,
		negRiskABI:      negRiskABI,
		proxyFactoryABI: proxyFactoryABI,
	}

	return client, nil
}

// RedeemPositionInfo represents a single position to redeem
type RedeemPositionInfo struct {
	ConditionID types.Keccak256
	Amounts     []float64
	NegRisk     bool
}

// RedeemPositions redeems multiple positions into USDC in a single batch transaction
func (c *GaslessWeb3Client) RedeemPositions(
	positions []RedeemPositionInfo,
) (*types.TransactionReceipt, error) {
	if len(positions) == 0 {
		return nil, fmt.Errorf("no positions to redeem")
	}

	// Build proxy transactions for each position
	proxyTxns := make([]map[string]interface{}, 0, len(positions))

	for i, pos := range positions {
		// Validate amounts
		if len(pos.Amounts) == 0 {
			return nil, fmt.Errorf("position %d: amounts array is empty", i)
		}

		// For Negative Risk markets, validate amounts according to Python implementation:
		// - WCOL Aggregator allows amounts array with zeros (at least one must be > 0)
		// - This matches Python's group_wcol_amounts which allows amount0 or amount1 to be 0
		if pos.NegRisk {
			hasPositiveAmount := false
			for j, amount := range pos.Amounts {
				if amount < 0 {
					return nil, fmt.Errorf("position %d: Negative Risk market amounts cannot be negative, but amounts[%d]=%f", i, j, amount)
				}
				if amount > 0 {
					hasPositiveAmount = true
				}
			}
			if !hasPositiveAmount {
				return nil, fmt.Errorf("position %d: Negative Risk market requires at least one positive amount", i)
			}
		}

		// Convert amounts to int (multiply by 1e6) - 使用big.Int避免精度损失
		intAmounts := make([]*big.Int, len(pos.Amounts))
		for j, amount := range pos.Amounts {
			// 使用big.Float进行精确计算，避免int64溢出和精度损失
			amountFloat := big.NewFloat(amount)
			multiplier := big.NewFloat(1e6)
			result := new(big.Float).Mul(amountFloat, multiplier)
			intAmount, _ := result.Int(nil)
			if intAmount.Sign() < 0 {
				return nil, fmt.Errorf("position %d: amount[%d] resulted in negative value: %f", i, j, amount)
			}
			intAmounts[j] = intAmount
		}

		// Determine contract address and encode data
		var to common.Address
		var data []byte
		var err error

		if pos.NegRisk {
			// Use neg risk adapter
			to = common.HexToAddress(internal.PolygonNegRiskAdapter)
			data, err = c.encodeRedeemNegRisk(pos.ConditionID, intAmounts)
		} else {
			// Use conditional tokens
			to = common.HexToAddress(internal.PolygonConditionalTokens)
			data, err = c.encodeRedeem(pos.ConditionID)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to encode redeem for position %d (conditionID: %s, negRisk: %v): %w",
				i, string(pos.ConditionID), pos.NegRisk, err)
		}

		// Add to proxy transactions
		proxyTxns = append(proxyTxns, map[string]interface{}{
			"typeCode": 1,
			"to":       to.Hex(),
			"value":    0,
			"data":     "0x" + hex.EncodeToString(data),
		})
	}

	// Execute batch transaction via gasless relay
	return c.executeGaslessBatch(proxyTxns, "Redeem Positions", "redeem")
}

// encodeRedeem encodes redeem positions transaction for regular markets
func (c *GaslessWeb3Client) encodeRedeem(conditionID types.Keccak256) ([]byte, error) {
	usdcAddr := common.HexToAddress(internal.PolygonCollateral)
	hashZero := common.HexToHash(internal.HashZero)
	indexSets := []*big.Int{big.NewInt(1), big.NewInt(2)}

	// redeemPositions(address collateralToken, bytes32 parentCollectionId, bytes32 conditionId, uint256[] indexSets)
	return c.conditionalABI.Pack("redeemPositions", usdcAddr, hashZero, common.HexToHash(string(conditionID)), indexSets)
}

// encodeRedeemNegRisk encodes redeem positions transaction for neg risk markets
// According to Python implementation, Negative Risk markets use WCOL Aggregator
// with selector 0xdbeccb23, not the redeemPositions function
// Function signature: function(bytes32 conditionId, uint256[] amounts)
// Data format: selector(4) + conditionId(32) + offset(32) + arrayLength(32) + amounts...
func (c *GaslessWeb3Client) encodeRedeemNegRisk(conditionID types.Keccak256, amounts []*big.Int) ([]byte, error) {
	// Use manual encoding to match Python's build_wcol_call_data format
	// Python: selector + encode_bytes32(conditionId) + encode_u256(0x40) + encode_u256(2) + encode_u256(amount0) + encode_u256(amount1)

	selector, err := hex.DecodeString("dbeccb23")
	if err != nil {
		return nil, fmt.Errorf("failed to decode selector: %w", err)
	}

	// Encode conditionId (32 bytes)
	conditionHash := common.HexToHash(string(conditionID))
	conditionBytes := conditionHash.Bytes()

	// Encode offset 0x40 (32 bytes, big-endian)
	offset := big.NewInt(0x40)
	offsetBytes := make([]byte, 32)
	offset.FillBytes(offsetBytes)

	// Encode array length (32 bytes, big-endian)
	arrayLen := big.NewInt(int64(len(amounts)))
	arrayLenBytes := make([]byte, 32)
	arrayLen.FillBytes(arrayLenBytes)

	// Encode amounts (each 32 bytes, big-endian)
	amountsBytes := make([]byte, 0, len(amounts)*32)
	for _, amount := range amounts {
		amountBytes := make([]byte, 32)
		amount.FillBytes(amountBytes)
		amountsBytes = append(amountsBytes, amountBytes...)
	}

	// Combine all parts
	data := append(selector, conditionBytes...)
	data = append(data, offsetBytes...)
	data = append(data, arrayLenBytes...)
	data = append(data, amountsBytes...)

	return data, nil
}

// executeGaslessBatch executes multiple transactions in a single batch via gasless relay
func (c *GaslessWeb3Client) executeGaslessBatch(
	proxyTxns []map[string]interface{},
	operationName string,
	metadata string,
) (*types.TransactionReceipt, error) {
	if len(proxyTxns) == 0 {
		return nil, fmt.Errorf("no transactions to execute")
	}

	var body interface{}
	var err error

	switch c.signatureType {
	case types.ProxySignatureType:
		body, err = c.buildProxyRelayTransactionBatch(proxyTxns, metadata)
	case types.SafeSignatureType:
		// Convert proxyTxns to Safe transaction format
		safeTxns := make([]map[string]interface{}, len(proxyTxns))
		for i, proxyTxn := range proxyTxns {
			safeTxns[i] = map[string]interface{}{
				"to":        proxyTxn["to"],
				"data":      proxyTxn["data"],
				"operation": 0, // Call operation (not DelegateCall for individual transactions)
				"value":     proxyTxn["value"],
			}
		}
		body, err = c.buildSafeRelayTransactionBatch(safeTxns, metadata)
	default:
		return nil, fmt.Errorf("unsupported signature type: %d", c.signatureType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to build relay transaction: %w", err)
	}

	// Format JSON body with spaces to match Python's json.dumps format
	bodyJSON, err := formatJSONWithSpaces(body)
	if err != nil {
		return nil, fmt.Errorf("failed to format body: %w", err)
	}

	// Sign request using LocalSigner
	requestHeaders, err := c.localSigner.SignRequest("POST", "/submit", body)
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	// Submit to relay
	requestURL := fmt.Sprintf("%s/submit", c.relayURL)
	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	for k, v := range requestHeaders {
		req.Header.Set(k, v)
	}

	// 记录 relayer 调用次数
	callCount := atomic.AddInt64(&c.relayerCallCount, 1)
	log.Printf("[Relayer调用 #%d] 批量提交交易到 relayer (类型: %d, 交易数: %d)", callCount, int(c.signatureType), len(proxyTxns))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// 限制错误日志长度，避免泄露敏感信息
		errorMsg := string(bodyBytes)
		if len(errorMsg) > 200 {
			errorMsg = errorMsg[:200] + "..."
		}
		log.Printf("[ERROR] [Relayer调用 #%d] 批量提交失败: HTTP %d", callCount, resp.StatusCode)
		return nil, fmt.Errorf("relay returned error: HTTP %d: %s", resp.StatusCode, errorMsg)
	}

	// Parse response
	var gaslessResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&gaslessResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	txHashStr, ok := gaslessResp["transactionHash"].(string)
	if !ok {
		return nil, fmt.Errorf("no transaction hash in response: %v", gaslessResp)
	}

	log.Printf("[OK] [Relayer调用 #%d] 批量提交成功，交易哈希: %s", callCount, txHashStr)

	// Wait for transaction receipt
	txHash := common.HexToHash(txHashStr)
	receipt, err := c.waitForTransactionReceipt(txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for receipt: %w", err)
	}

	return receipt, nil
}

// formatJSONWithSpaces formats JSON with spaces to match Python's json.dumps format
func formatJSONWithSpaces(body interface{}) ([]byte, error) {
	bodyJSONCompact, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	bodyJSONStr := string(bodyJSONCompact)
	bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)
	bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)
	return []byte(bodyJSONStr), nil
}

// signSafeTransaction signs a Safe transaction
func (c *GaslessWeb3Client) signSafeTransaction(
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

	txHashBytes, err := c.client.CallContract(context.Background(), callMsg, nil)
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
func (c *GaslessWeb3Client) getSafeProxyAddress() (types.EthAddress, error) {
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

	result, err := c.client.CallContract(context.Background(), callMsg, nil)
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

// getSafeProxyFactoryABI returns a minimal ABI for the SafeProxyFactory contract
// Only includes computeProxyAddress function
func getSafeProxyFactoryABI() (*abi.ABI, error) {
	abiJSON := `[
		{
			"inputs": [
				{"internalType": "address", "name": "owner", "type": "address"}
			],
			"name": "computeProxyAddress",
			"outputs": [
				{"internalType": "address", "name": "", "type": "address"}
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

// getSafeMultiSendABI returns a minimal ABI for the Safe multiSend contract
// Only includes multiSend function
func getSafeMultiSendABI() (*abi.ABI, error) {
	abiJSON := `[
		{
			"inputs": [
				{"internalType": "bytes", "name": "transactions", "type": "bytes"}
			],
			"name": "multiSend",
			"outputs": [],
			"stateMutability": "payable",
			"type": "function"
		}
	]`

	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}

	return &parsedABI, nil
}

// buildProxyRelayTransactionBatch builds a proxy relay transaction body for batch transactions
// Returns a struct to maintain field order matching Python version
func (c *GaslessWeb3Client) buildProxyRelayTransactionBatch(
	proxyTxns []map[string]interface{},
	metadata string,
) (*ProxyRelayBody, error) {
	if len(proxyTxns) == 0 {
		return nil, fmt.Errorf("no transactions to batch")
	}

	// Get relay nonce
	nonce, err := c.getRelayNonce("PROXY")
	if err != nil {
		return nil, fmt.Errorf("failed to get relay nonce: %w", err)
	}

	gasPrice := "0"
	relayerFee := "0"

	// Encode proxy transactions (batch)
	encodedTxn, err := c.encodeProxy(proxyTxns)
	if err != nil {
		return nil, fmt.Errorf("failed to encode proxy transactions: %w", err)
	}

	// Estimate gas
	proxyFactoryAddr := common.HexToAddress(internal.PolygonProxyFactory)
	callMsg := ethereum.CallMsg{
		From: common.HexToAddress(string(c.baseAddress)),
		To:   &proxyFactoryAddr,
		Data: encodedTxn,
	}

	estimatedGas, err := c.client.EstimateGas(context.Background(), callMsg)
	if err != nil {
		// Use default if estimation fails (increase for batch)
		estimatedGas = internal.DefaultGasEstimate * uint64(len(proxyTxns))
	}

	gasLimit := strconv.FormatUint(estimatedGas*internal.GasEstimateMultiplier/100+internal.GasEstimateExtra, 10) // 1.3x + 100k

	// Create proxy struct for signing
	encodedTxnHex := "0x" + hex.EncodeToString(encodedTxn)

	// Step 1: Create raw struct bytes (not hashed yet)
	structBytes, err := c.createProxyStructBytes(
		string(c.baseAddress),
		proxyFactoryAddr.Hex(),
		encodedTxnHex,
		relayerFee,
		gasPrice,
		gasLimit,
		strconv.Itoa(nonce),
		c.relayHub,
		c.relayAddress,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy struct bytes: %w", err)
	}

	// Step 2: Hash the struct bytes (Python: self.w3.keccak(struct))
	structHashBytes := crypto.Keccak256(structBytes)

	// Step 3: Convert to hex string with 0x prefix (Python: "0x" + struct_hash.hex())
	structHashHex := "0x" + hex.EncodeToString(structHashBytes)

	// Step 4: Sign using encode_defunct(hexstr=struct_hash)
	structHashBytesFromHex, err := hex.DecodeString(strings.TrimPrefix(structHashHex, "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to decode struct hash hex: %w", err)
	}

	// Create encode_defunct message: "\x19Ethereum Signed Message:\n" + len(bytes) + bytes
	messagePrefix := []byte("\x19Ethereum Signed Message:\n")
	messageLenStr := []byte(strconv.Itoa(len(structHashBytesFromHex)))
	messageBytes := append(messagePrefix, messageLenStr...)
	messageBytes = append(messageBytes, structHashBytesFromHex...)

	// Hash the message (sign_message does keccak256)
	hash := crypto.Keccak256Hash(messageBytes)
	signature, err := c.signer.SignWithRecovery(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Get proxy address (must succeed for request body)
	proxyAddr, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy address: %w", err)
	}

	// Use struct to maintain field order matching Python version for consistent HMAC signature
	return &ProxyRelayBody{
		Data:        encodedTxnHex,
		From:        string(c.baseAddress),
		Metadata:    metadata,
		Nonce:       strconv.Itoa(nonce),
		ProxyWallet: string(proxyAddr),
		Signature:   "0x" + hex.EncodeToString(signature),
		SignatureParams: SignatureParams{
			GasPrice:   gasPrice,
			GasLimit:   gasLimit,
			RelayerFee: relayerFee,
			RelayHub:   c.relayHub,
			Relay:      c.relayAddress,
		},
		To:   proxyFactoryAddr.Hex(),
		Type: "PROXY",
	}, nil
}

// createSafeMultiSendTransaction creates a Safe multiSend transaction that batches multiple transactions
// This matches Python's create_safe_multisend_transaction function
func (c *GaslessWeb3Client) createSafeMultiSendTransaction(
	txns []map[string]interface{},
) (common.Address, []byte, error) {
	if len(txns) == 0 {
		return common.Address{}, nil, fmt.Errorf("no transactions to batch")
	}

	// If only one transaction, return it directly (no need for multiSend)
	if len(txns) == 1 {
		txn := txns[0]
		to := common.HexToAddress(txn["to"].(string))
		dataHex := txn["data"].(string)
		data, err := hex.DecodeString(strings.TrimPrefix(dataHex, "0x"))
		if err != nil {
			return common.Address{}, nil, fmt.Errorf("failed to decode data: %w", err)
		}
		return to, data, nil
	}

	// Pack each transaction: [uint8 operation, address to, uint256 value, uint256 dataLength, bytes data]
	encodedTxns := make([][]byte, 0, len(txns))
	for _, txn := range txns {
		operation := uint8(txn["operation"].(int))
		to := common.HexToAddress(txn["to"].(string))
		value := big.NewInt(int64(txn["value"].(int)))
		dataHex := txn["data"].(string)
		data, err := hex.DecodeString(strings.TrimPrefix(dataHex, "0x"))
		if err != nil {
			return common.Address{}, nil, fmt.Errorf("failed to decode data: %w", err)
		}

		// Pack: [uint8, address (20 bytes), uint256 (32 bytes), uint256 (32 bytes), bytes]
		packed := make([]byte, 0)
		// uint8 operation (1 byte)
		packed = append(packed, byte(operation))
		// address to (20 bytes)
		packed = append(packed, to.Bytes()...)
		// uint256 value (32 bytes, padded)
		packed = append(packed, pad32Bytes(value.Bytes())...)
		// uint256 dataLength (32 bytes, padded)
		dataLength := big.NewInt(int64(len(data)))
		packed = append(packed, pad32Bytes(dataLength.Bytes())...)
		// bytes data (variable length)
		packed = append(packed, data...)

		encodedTxns = append(encodedTxns, packed)
	}

	// Concatenate all encoded transactions
	concatenatedTxns := bytes.Join(encodedTxns, nil)

	// Get multiSend ABI
	multiSendABI, err := getSafeMultiSendABI()
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to get multiSend ABI: %w", err)
	}

	// Encode multiSend(bytes) call
	// multiSend(bytes memory transactions)
	multiSendData, err := multiSendABI.Pack("multiSend", concatenatedTxns)
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("failed to pack multiSend: %w", err)
	}

	// Return multiSend contract address and encoded data
	multiSendAddr := common.HexToAddress(internal.SafeMultiSend)
	return multiSendAddr, multiSendData, nil
}

// buildSafeRelayTransactionBatch builds a Safe relay transaction body for batch transactions
// Uses Safe multiSend contract to batch multiple transactions into a single Safe transaction
// Returns a struct to maintain field order matching Python version
func (c *GaslessWeb3Client) buildSafeRelayTransactionBatch(
	safeTxns []map[string]interface{},
	metadata string,
) (*SafeRelayBody, error) {
	if len(safeTxns) == 0 {
		return nil, fmt.Errorf("no transactions to batch")
	}

	// Get relay nonce for Safe wallet
	nonce, err := c.getRelayNonce("SAFE")
	if err != nil {
		return nil, fmt.Errorf("failed to get relay nonce: %w", err)
	}

	// Create multiSend transaction (or use single transaction if only one)
	to, data, err := c.createSafeMultiSendTransaction(safeTxns)
	if err != nil {
		return nil, fmt.Errorf("failed to create safe multiSend transaction: %w", err)
	}

	// Determine operation: DelegateCall (1) for multiSend, Call (0) for single transaction
	operation := 0
	if len(safeTxns) > 1 {
		operation = 1 // DelegateCall for multiSend
	}

	// Build Safe transaction
	safeTxn := map[string]interface{}{
		"to":        to.Hex(),
		"data":      "0x" + hex.EncodeToString(data),
		"operation": operation,
		"value":     0,
	}

	// Sign Safe transaction
	signature, err := c.signSafeTransaction(safeTxn, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to sign safe transaction: %w", err)
	}

	// Adjust signature recovery ID (matching Python logic)
	sigHex := hex.EncodeToString(signature)
	lastTwo := sigHex[len(sigHex)-2:]
	switch lastTwo {
	case "00", "1b":
		sigHex = sigHex[:len(sigHex)-2] + "1f"
	case "01", "1c":
		sigHex = sigHex[:len(sigHex)-2] + "20"
	}

	// Get Safe proxy address
	safeProxyAddr, err := c.getSafeProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("failed to get safe proxy address: %w", err)
	}

	// Use struct to maintain field order matching Python version for consistent HMAC signature
	return &SafeRelayBody{
		Data:        safeTxn["data"].(string),
		From:        string(c.baseAddress),
		Metadata:    metadata,
		Nonce:       strconv.Itoa(nonce),
		ProxyWallet: string(safeProxyAddr),
		Signature:   "0x" + sigHex,
		SignatureParams: SafeSignatureParams{
			BaseGas:        "0",
			GasPrice:       "0",
			GasToken:       internal.ZeroAddress,
			Operation:      strconv.Itoa(operation),
			RefundReceiver: internal.ZeroAddress,
			SafeTxnGas:     "0",
		},
		To:   to.Hex(),
		Type: "SAFE",
	}, nil
}

// encodeProxy encodes a proxy transaction (supports single or multiple transactions)
func (c *GaslessWeb3Client) encodeProxy(proxyTxns []map[string]interface{}) ([]byte, error) {
	// proxy((uint8 typeCode, address to, uint256 value, bytes data)[] transactions)
	// Pack transactions as array
	transactions := make([]struct {
		TypeCode uint8
		To       common.Address
		Value    *big.Int
		Data     []byte
	}, len(proxyTxns))

	for i, proxyTxn := range proxyTxns {
		typeCode := uint8(proxyTxn["typeCode"].(int))
		to := common.HexToAddress(proxyTxn["to"].(string))
		value := big.NewInt(int64(proxyTxn["value"].(int)))
		dataHex := proxyTxn["data"].(string)
		data, err := hex.DecodeString(strings.TrimPrefix(dataHex, "0x"))
		if err != nil {
			return nil, fmt.Errorf("failed to decode data for transaction %d: %w", i, err)
		}

		transactions[i] = struct {
			TypeCode uint8
			To       common.Address
			Value    *big.Int
			Data     []byte
		}{
			TypeCode: typeCode,
			To:       to,
			Value:    value,
			Data:     data,
		}
	}

	return c.proxyFactoryABI.Pack("proxy", transactions)
}

// createProxyStructBytes creates raw proxy struct bytes (not hashed)
// This matches Python's create_proxy_struct function
func (c *GaslessWeb3Client) createProxyStructBytes(
	from string,
	to string,
	data string,
	txFee string,
	gasPrice string,
	gasLimit string,
	nonce string,
	relayHub string,
	relay string,
) ([]byte, error) {
	relayHubPrefix := []byte("rlx:")

	fromBytes, err := hex.DecodeString(strings.TrimPrefix(from, "0x"))
	if err != nil {
		return nil, err
	}

	toBytes, err := hex.DecodeString(strings.TrimPrefix(to, "0x"))
	if err != nil {
		return nil, err
	}

	dataBytes, err := hex.DecodeString(strings.TrimPrefix(data, "0x"))
	if err != nil {
		return nil, err
	}

	txFeeInt, _ := new(big.Int).SetString(txFee, 10)
	gasPriceInt, _ := new(big.Int).SetString(gasPrice, 10)
	gasLimitInt, _ := new(big.Int).SetString(gasLimit, 10)
	nonceInt, _ := new(big.Int).SetString(nonce, 10)

	relayHubBytes, err := hex.DecodeString(strings.TrimPrefix(relayHub, "0x"))
	if err != nil {
		return nil, err
	}

	relayBytes, err := hex.DecodeString(strings.TrimPrefix(relay, "0x"))
	if err != nil {
		return nil, err
	}

	structBytes := append(relayHubPrefix, fromBytes...)
	structBytes = append(structBytes, toBytes...)
	structBytes = append(structBytes, dataBytes...)
	structBytes = append(structBytes, pad32Bytes(txFeeInt.Bytes())...)
	structBytes = append(structBytes, pad32Bytes(gasPriceInt.Bytes())...)
	structBytes = append(structBytes, pad32Bytes(gasLimitInt.Bytes())...)
	structBytes = append(structBytes, pad32Bytes(nonceInt.Bytes())...)
	structBytes = append(structBytes, relayHubBytes...)
	structBytes = append(structBytes, relayBytes...)

	// Return raw struct bytes (not hashed) - matching Python's create_proxy_struct
	return structBytes, nil
}

// pad32Bytes pads bytes to 32 bytes
func pad32Bytes(b []byte) []byte {
	if len(b) >= 32 {
		return b[:32]
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}

// getRelayNonce gets nonce from relay with retry mechanism
func (c *GaslessWeb3Client) getRelayNonce(walletType string) (int, error) {
	url := fmt.Sprintf("%s/nonce", c.relayURL)

	// Retry up to 3 times with exponential backoff
	maxRetries := internal.RelayNonceMaxRetries
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			time.Sleep(backoff)
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = err
			continue
		}

		q := req.URL.Query()
		q.Set("address", string(c.baseAddress))
		q.Set("type", walletType)
		req.URL.RawQuery = q.Encode()

		// Create context with timeout for this specific request
		ctx, cancel := context.WithTimeout(context.Background(), internal.RelayNonceTimeout)
		req = req.WithContext(ctx)

		// 记录 relayer 调用次数（nonce 请求）
		callCount := atomic.AddInt64(&c.relayerCallCount, 1)
		log.Printf("[Relayer调用 #%d] 获取 nonce (类型: %s, 尝试: %d/%d)", callCount, walletType, attempt+1, maxRetries)

		resp, err := c.httpClient.Do(req)
		cancel()

		if err != nil {
			lastErr = fmt.Errorf("failed to get nonce (attempt %d/%d): %w", attempt+1, maxRetries, err)
			// Check if it's a timeout error - if so, retry
			if ctx.Err() == context.DeadlineExceeded || strings.Contains(err.Error(), "timeout") {
				continue
			}
			// For other errors, return immediately
			return 0, lastErr
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("relay returned error: HTTP %d", resp.StatusCode)
			continue // Retry on non-200 status
		}

		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			lastErr = fmt.Errorf("failed to decode response: %w", err)
			continue // Retry on decode error
		}

		nonceStr, ok := result["nonce"].(string)
		if !ok {
			nonceFloat, ok := result["nonce"].(float64)
			if !ok {
				lastErr = fmt.Errorf("invalid nonce in response: %v", result)
				continue // Retry on invalid response
			}
			nonce := int(nonceFloat)
			log.Printf("[OK] [Relayer调用 #%d] 成功获取 nonce: %d (类型: %s)", callCount, nonce, walletType)
			return nonce, nil
		}

		nonce, err := strconv.Atoi(nonceStr)
		if err != nil {
			lastErr = fmt.Errorf("failed to parse nonce: %w", err)
			continue // Retry on parse error
		}

		// Success!
		log.Printf("[OK] [Relayer调用 #%d] 成功获取 nonce: %d (类型: %s)", callCount, nonce, walletType)
		return nonce, nil
	}

	// All retries failed
	return 0, fmt.Errorf("failed to get nonce after %d attempts: %w", maxRetries, lastErr)
}

// waitForTransactionReceipt waits for a transaction receipt
func (c *GaslessWeb3Client) waitForTransactionReceipt(txHash common.Hash) (*types.TransactionReceipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), internal.TransactionWaitTimeout)
	defer cancel()

	for {
		receipt, err := c.client.TransactionReceipt(ctx, txHash)
		if err == nil {
			// Check if transaction failed
			if receipt.Status == 0 {
				// Transaction failed, try to extract error message
				errorMsg := c.extractTransactionError(ctx, txHash, receipt)
				if errorMsg != "" {
					return nil, fmt.Errorf("交易执行失败 (txHash: %s): %s", txHash.Hex(), errorMsg)
				}
				return nil, fmt.Errorf("交易执行失败 (txHash: %s): 状态为失败，但无法提取错误信息", txHash.Hex())
			}

			// Get transaction to extract To address
			tx, _, err := c.client.TransactionByHash(ctx, txHash)
			var toAddr *common.Address
			if err == nil && tx != nil {
				toAddr = tx.To()
			}
			// Convert to our receipt type
			return convertReceipt(receipt, toAddr), nil
		}

		if err == ethereum.NotFound {
			// Wait and retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(internal.TransactionDelay):
				continue
			}
		}

		return nil, err
	}
}

// extractTransactionError 尝试从失败的交易中提取错误信息
func (c *GaslessWeb3Client) extractTransactionError(ctx context.Context, txHash common.Hash, receipt *ethtypes.Receipt) string {
	// 获取交易详情
	tx, _, err := c.client.TransactionByHash(ctx, txHash)
	if err != nil {
		// 如果无法获取交易，至少提供基本信息
		return fmt.Sprintf("无法获取交易详情 (txHash: %s)", txHash.Hex())
	}

	// 尝试通过 eth_call 模拟交易来获取 revert reason
	// 注意：这需要知道交易执行时的区块状态，可能不总是准确
	msg := ethereum.CallMsg{
		To:       tx.To(),
		Gas:      tx.Gas(),
		GasPrice: tx.GasPrice(),
		Value:    tx.Value(),
		Data:     tx.Data(),
	}

	// 在交易所在的区块之前执行 call（使用 receipt.BlockNumber - 1）
	blockNum := new(big.Int).Sub(receipt.BlockNumber, big.NewInt(1))
	_, err = c.client.CallContract(ctx, msg, blockNum)
	if err != nil {
		// 从错误信息中提取 revert reason
		errStr := err.Error()

		// 检查是否是 GS026 错误
		if strings.Contains(errStr, "GS026") {
			return fmt.Sprintf("Gnosis Safe 错误 GS026: 交易签名验证失败。根据 https://ethereum.stackexchange.com/questions/143583，这通常是因为签名版本(v值)不正确。当使用 personal_sign 时，v 值需要加 4 (27->31/0x1f, 28->32/0x20)。可能原因：1) 签名 v 值不正确 2) 签名者不是 Safe 的 owner 3) 交易参数不匹配 4) nonce 不匹配。交易哈希: %s", txHash.Hex())
		}

		// 尝试提取 revert reason（如果错误信息中包含）
		if strings.Contains(errStr, "revert") || strings.Contains(errStr, "execution reverted") {
			// 尝试提取错误消息
			re := regexp.MustCompile(`(?:revert|execution reverted)(?::\s*)?([^"]+)`)
			matches := re.FindStringSubmatch(errStr)
			if len(matches) > 1 {
				errorDetail := matches[1]
				// 检查是否包含 GS026
				if strings.Contains(errorDetail, "GS026") {
					return fmt.Sprintf("Gnosis Safe 错误 GS026: %s (txHash: %s)。可能原因：签名验证失败、权限不足或交易参数不匹配", errorDetail, txHash.Hex())
				}
				return fmt.Sprintf("交易回滚: %s (txHash: %s)", errorDetail, txHash.Hex())
			}
			return fmt.Sprintf("交易回滚: %s (txHash: %s)", errStr, txHash.Hex())
		}

		return fmt.Sprintf("交易失败: %s (txHash: %s)", errStr, txHash.Hex())
	}

	// 如果 call 成功但交易失败，可能是其他原因
	// 检查交易是否发送到 Safe 合约
	if tx.To() != nil {
		toAddr := tx.To().Hex()
		// 检查是否是 Safe 相关错误（通过检查交易目标地址）
		return fmt.Sprintf("交易执行失败，但无法通过模拟调用提取错误信息。交易目标: %s, txHash: %s。如果是 Safe 交易，可能是 GS026 签名验证错误", toAddr, txHash.Hex())
	}

	return fmt.Sprintf("交易执行失败 (txHash: %s)", txHash.Hex())
}

// convertReceipt converts ethereum receipt to our receipt type
func convertReceipt(receipt *ethtypes.Receipt, toAddr *common.Address) *types.TransactionReceipt {
	status := 0
	if receipt.Status == 1 {
		status = 1
	}

	var to *types.EthAddress
	if toAddr != nil {
		addr := types.EthAddress(toAddr.Hex())
		to = &addr
	}

	logs := make([]types.Log, len(receipt.Logs))
	for i, log := range receipt.Logs {
		topics := make([]types.Keccak256, len(log.Topics))
		for j, topic := range log.Topics {
			topics[j] = types.Keccak256(topic.Hex())
		}
		logs[i] = types.Log{
			Address: types.EthAddress(log.Address.Hex()),
			Topics:  topics,
			Data:    "0x" + hex.EncodeToString(log.Data),
		}
	}

	// Get transaction to extract From address
	// Note: receipt doesn't have From, we'll need to get it from the transaction
	// For now, we'll use empty address
	from := types.EthAddress("")

	return &types.TransactionReceipt{
		TxHash:            types.Keccak256(receipt.TxHash.Hex()),
		BlockNumber:       receipt.BlockNumber.Uint64(),
		BlockHash:         types.Keccak256(receipt.BlockHash.Hex()),
		Status:            status,
		GasUsed:           receipt.GasUsed,
		EffectiveGasPrice: receipt.EffectiveGasPrice.String(),
		From:              from,
		To:                to,
		Logs:              logs,
	}
}

// Helper functions to get ABIs
func getConditionalTokensABI() (*abi.ABI, error) {
	// Minimal ABI for redeemPositions
	abiJSON := `[{
		"inputs": [
			{"internalType": "address", "name": "collateralToken", "type": "address"},
			{"internalType": "bytes32", "name": "parentCollectionId", "type": "bytes32"},
			{"internalType": "bytes32", "name": "conditionId", "type": "bytes32"},
			{"internalType": "uint256[]", "name": "indexSets", "type": "uint256[]"}
		],
		"name": "redeemPositions",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &parsedABI, nil
}

func getNegRiskAdapterABI() (*abi.ABI, error) {
	// Minimal ABI for redeemPositions
	abiJSON := `[{
		"inputs": [
			{"internalType": "bytes32", "name": "_conditionId", "type": "bytes32"},
			{"internalType": "uint256[]", "name": "_amounts", "type": "uint256[]"}
		],
		"name": "redeemPositions",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &parsedABI, nil
}

func getProxyFactoryABI() (*abi.ABI, error) {
	// Minimal ABI for proxy
	abiJSON := `[{
		"inputs": [{
			"components": [
				{"internalType": "uint8", "name": "typeCode", "type": "uint8"},
				{"internalType": "address", "name": "to", "type": "address"},
				{"internalType": "uint256", "name": "value", "type": "uint256"},
				{"internalType": "bytes", "name": "data", "type": "bytes"}
			],
			"internalType": "struct ProxyWalletFactory.Transaction[]",
			"name": "transactions",
			"type": "tuple[]"
		}],
		"name": "proxy",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`
	parsedABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	return &parsedABI, nil
}
