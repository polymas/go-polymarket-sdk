package web3

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
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// executeGaslessBatch executes multiple transactions in a single batch via gasless relay
func (c *GaslessClient) executeGaslessBatch(
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

	// 记录 relayer 调用次数（在格式化 body 之后，用于日志）
	callCount := atomic.AddInt64(&c.relayerCallCount, 1)

	// Debug: log request body (truncated for security)
	if len(bodyJSON) > 500 {
		log.Printf("[DEBUG] [Relayer调用 #%d] 请求体 (前500字符): %s...", callCount, string(bodyJSON[:500]))
	} else {
		log.Printf("[DEBUG] [Relayer调用 #%d] 请求体: %s", callCount, string(bodyJSON))
	}
	
	// Debug: log encoded proxy data length and first bytes
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(bodyJSON, &bodyMap); err == nil {
		if encodedTxnHex, ok := bodyMap["data"].(string); ok {
			previewLen := 100
			if len(encodedTxnHex) < previewLen {
				previewLen = len(encodedTxnHex)
			}
			log.Printf("[DEBUG] [Relayer调用 #%d] Proxy data length: %d bytes, first %d chars: %s", 
				callCount, len(encodedTxnHex), previewLen, encodedTxnHex[:previewLen])
		}
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

	// 检查交易状态
	if state, ok := gaslessResp["state"].(string); ok {
		if state == "STATE_FAILED" || state == "FAILED" || state == "failed" {
			// 尝试提取错误信息 - 检查多个可能的字段
			errorMsg := "交易提交失败"
			errorDetails := []string{}
			
			// 检查各种可能的错误字段
			if errMsg, ok := gaslessResp["error"].(string); ok && errMsg != "" {
				errorMsg = errMsg
			} else if errMsg, ok := gaslessResp["message"].(string); ok && errMsg != "" {
				errorMsg = errMsg
			} else if errMsg, ok := gaslessResp["errorMessage"].(string); ok && errMsg != "" {
				errorMsg = errMsg
			} else if errMsg, ok := gaslessResp["reason"].(string); ok && errMsg != "" {
				errorMsg = errMsg
			} else if errData, ok := gaslessResp["error"].(map[string]interface{}); ok {
				// 错误可能是嵌套对象
				if msg, ok := errData["message"].(string); ok {
					errorMsg = msg
				}
				if code, ok := errData["code"].(string); ok {
					errorDetails = append(errorDetails, fmt.Sprintf("code: %s", code))
				}
			}
			
			// 检查是否有嵌套的错误信息
			if details, ok := gaslessResp["details"].(map[string]interface{}); ok {
				for k, v := range details {
					errorDetails = append(errorDetails, fmt.Sprintf("%s: %v", k, v))
				}
			}

			transactionID := ""
			if txID, ok := gaslessResp["transactionID"].(string); ok {
				transactionID = txID
			}

			fullErrorMsg := errorMsg
			if len(errorDetails) > 0 {
				fullErrorMsg = fmt.Sprintf("%s (%s)", errorMsg, strings.Join(errorDetails, ", "))
			}

			log.Printf("[ERROR] [Relayer调用 #%d] 交易提交失败 (state: %s, transactionID: %s): %s",
				callCount, state, transactionID, fullErrorMsg)
			log.Printf("[ERROR] [Relayer调用 #%d] 完整响应 (JSON): %s", callCount, formatMapAsJSON(gaslessResp))
			return nil, fmt.Errorf("交易提交失败 (state: %s, transactionID: %s): %s", state, transactionID, fullErrorMsg)
		}
	}

	txHashStr, ok := gaslessResp["transactionHash"].(string)
	if !ok {
		// 尝试其他可能的字段名
		if txHash, ok2 := gaslessResp["txHash"].(string); ok2 {
			txHashStr = txHash
		} else if txHash, ok2 := gaslessResp["hash"].(string); ok2 {
			txHashStr = txHash
		} else {
			// 如果状态不是失败但没有交易哈希，可能是还在处理中
			if state, ok := gaslessResp["state"].(string); ok && state != "STATE_FAILED" {
				log.Printf("[WARN] [Relayer调用 #%d] 响应中没有找到交易哈希，但状态为: %s，响应内容: %+v", callCount, state, gaslessResp)
				return nil, fmt.Errorf("交易可能还在处理中，未返回交易哈希 (state: %s): %v", state, gaslessResp)
			}
			log.Printf("[ERROR] [Relayer调用 #%d] 响应中没有找到交易哈希，响应内容: %+v", callCount, gaslessResp)
			return nil, fmt.Errorf("no transaction hash in response: %v", gaslessResp)
		}
	}

	if txHashStr == "" {
		// 检查状态，如果失败则返回错误
		if state, ok := gaslessResp["state"].(string); ok && (state == "STATE_FAILED" || state == "FAILED" || state == "failed") {
			errorMsg := "交易提交失败"
			if errMsg, ok := gaslessResp["error"].(string); ok && errMsg != "" {
				errorMsg = errMsg
			}
			log.Printf("[ERROR] [Relayer调用 #%d] 交易哈希为空且状态为失败，响应内容: %+v", callCount, gaslessResp)
			return nil, fmt.Errorf("交易提交失败 (state: %s): %s", state, errorMsg)
		}
		log.Printf("[ERROR] [Relayer调用 #%d] 交易哈希为空，响应内容: %+v", callCount, gaslessResp)
		return nil, fmt.Errorf("transaction hash is empty in response: %v", gaslessResp)
	}

	log.Printf("[OK] [Relayer调用 #%d] 批量提交成功，交易哈希: %s", callCount, txHashStr)

	// Wait for transaction receipt
	txHash := common.HexToHash(txHashStr)
	receipt, err := c.waitForTransactionReceipt(txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for receipt: %w", err)
	}

	log.Printf("[OK] [Relayer调用 #%d] 交易已确认，区块号: %d", callCount, receipt.BlockNumber)
	return receipt, nil
}

// formatMapAsJSON formats a map as JSON string for logging
func formatMapAsJSON(m map[string]interface{}) string {
	jsonBytes, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", m)
	}
	return string(jsonBytes)
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

// buildProxyRelayTransactionBatch builds a proxy relay transaction body for batch transactions
// Returns a struct to maintain field order matching Python version
func (c *GaslessClient) buildProxyRelayTransactionBatch(
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

	estimatedGas, err := c.estimateGasWithRetry(context.Background(), callMsg)
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
func (c *GaslessClient) createSafeMultiSendTransaction(
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
func (c *GaslessClient) buildSafeRelayTransactionBatch(
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
// Manual encoding to match Rust implementation
// Function selector: proxy((uint8,address,uint256,bytes)[])
// Rust uses hardcoded selector: 0x415565b0
func (c *GaslessClient) encodeProxy(proxyTxns []map[string]interface{}) ([]byte, error) {
	// Function selector: proxy((uint8,address,uint256,bytes)[])
	// Rust hardcodes: 0x415565b0 (matching Rust implementation)
	selector, err := hex.DecodeString("415565b0")
	if err != nil {
		return nil, fmt.Errorf("failed to decode selector: %w", err)
	}

	var data []byte
	data = append(data, selector...)

	// Encode array offset (32 bytes) = 0x20
	arrayOffset := big.NewInt(0x20)
	offsetBytes := make([]byte, 32)
	arrayOffset.FillBytes(offsetBytes)
	data = append(data, offsetBytes...)

	// Encode array length (32 bytes)
	arrayLen := big.NewInt(int64(len(proxyTxns)))
	lenBytes := make([]byte, 32)
	arrayLen.FillBytes(lenBytes)
	data = append(data, lenBytes...)

	// Encode each transaction
	// Matching Rust: encode tuple offset first, then tuple data in same loop
	for i, proxyTxn := range proxyTxns {
		// Calculate tuple offset: 0x20 * (len + 1) + current data length
		// Rust: 0x20 * (proxy_txns.len() + 1) as u64 + data.len() as u64
		tupleOffsetValue := int64(0x20*(len(proxyTxns)+1) + len(data))
		tupleOffset := big.NewInt(tupleOffsetValue)
		tupleOffsetBytes := make([]byte, 32)
		tupleOffset.FillBytes(tupleOffsetBytes)
		data = append(data, tupleOffsetBytes...)
		
		log.Printf("[DEBUG] encodeProxy: Transaction %d, tuple offset: 0x%x (calculated from: 0x%x * %d + %d)", 
			i, tupleOffsetValue, 0x20, len(proxyTxns)+1, len(data)-32) // -32 because we just added the offset
		
		typeCode := uint8(proxyTxn["typeCode"].(int))
		to := common.HexToAddress(proxyTxn["to"].(string))
		value := big.NewInt(int64(proxyTxn["value"].(int)))
		dataHex := proxyTxn["data"].(string)
		txnData, err := hex.DecodeString(strings.TrimPrefix(dataHex, "0x"))
		if err != nil {
			return nil, fmt.Errorf("failed to decode data: %w", err)
		}
		
		// Debug: log transaction details
		log.Printf("[DEBUG] encodeProxy: Transaction %d details - typeCode: %d, to: %s, value: %d, dataLen: %d", 
			i, typeCode, to.Hex(), value.Int64(), len(txnData))

		// Encode typeCode (uint8, padded to 32 bytes)
		typeCodeBytes := make([]byte, 32)
		typeCodeBytes[31] = typeCode
		data = append(data, typeCodeBytes...)

		// Encode to (address, padded to 32 bytes)
		toBytes := make([]byte, 32)
		copy(toBytes[12:], to.Bytes())
		data = append(data, toBytes...)

		// Encode value (uint256, 32 bytes)
		valueBytes := make([]byte, 32)
		value.FillBytes(valueBytes)
		data = append(data, valueBytes...)

		// Encode data offset (32 bytes) = 0x60 (relative to tuple start)
		dataOffset := big.NewInt(0x60)
		dataOffsetBytes := make([]byte, 32)
		dataOffset.FillBytes(dataOffsetBytes)
		data = append(data, dataOffsetBytes...)

		// Encode data length (32 bytes)
		dataLen := big.NewInt(int64(len(txnData)))
		dataLenBytes := make([]byte, 32)
		dataLen.FillBytes(dataLenBytes)
		data = append(data, dataLenBytes...)

		// Encode data (variable length, no padding in Rust implementation)
		data = append(data, txnData...)
	}

	return data, nil
}

// createProxyStructBytes creates raw proxy struct bytes (not hashed)
// This matches Python's create_proxy_struct function
func (c *GaslessClient) createProxyStructBytes(
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
func (c *GaslessClient) getRelayNonce(walletType string) (int, error) {
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
func (c *GaslessClient) waitForTransactionReceipt(txHash common.Hash) (*types.TransactionReceipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), internal.TransactionWaitTimeout)
	defer cancel()

	startTime := time.Now()
	attemptCount := 0

	for {
		attemptCount++
		receipt, err := c.transactionReceiptWithRetry(ctx, txHash)
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
			tx, _, err := c.transactionByHashWithRetry(ctx, txHash)
			var toAddr *common.Address
			if err == nil && tx != nil {
				toAddr = tx.To()
			}
			// Convert to our receipt type
			return convertReceipt(receipt, toAddr), nil
		}

		if err == ethereum.NotFound {
			// Wait and retry
			elapsed := time.Since(startTime)
			if attemptCount%10 == 0 { // 每10次尝试打印一次日志
				log.Printf("[INFO] 等待交易确认中... (尝试 #%d, 已等待: %v, 交易哈希: %s)", attemptCount, elapsed, txHash.Hex())
			}
			select {
			case <-ctx.Done():
				log.Printf("[ERROR] 等待交易确认超时 (已等待: %v, 交易哈希: %s)", elapsed, txHash.Hex())
				return nil, ctx.Err()
			case <-time.After(internal.TransactionDelay):
				continue
			}
		}

		log.Printf("[ERROR] 获取交易收据失败: %v (交易哈希: %s)", err, txHash.Hex())
		return nil, err
	}
}

// extractTransactionError 尝试从失败的交易中提取错误信息
func (c *GaslessClient) extractTransactionError(ctx context.Context, txHash common.Hash, receipt *ethtypes.Receipt) string {
	// 获取交易详情
	tx, _, err := c.transactionByHashWithRetry(ctx, txHash)
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
	_, err = c.callContractWithRetry(ctx, msg, blockNum)
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
