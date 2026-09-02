package relayer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	sdkerrors "github.com/polymas/go-polymarket-sdk/errors"
	"github.com/polymas/go-polymarket-sdk/internal"
	sdkhttp "github.com/polymas/go-polymarket-sdk/internal/transport"
	"github.com/polymas/go-polymarket-sdk/types"
)

// CWIADeployBody 是 type=WALLET-CREATE 的提交体。
//
// 对照 py-builder-relayer-client/models.py:DepositWalletCreateRequest.to_dict()：
//
//	{
//	  "type": "WALLET-CREATE",
//	  "from": <EOA>,
//	  "to":   <DepositWalletFactory>
//	}
//
// 不需要 signature——relayer 自己以 operator 身份调
// factory.deploy([owner], [id])，操作权限来源是 operator 角色而不是用户签名。
type CWIADeployBody struct {
	Type string `json:"type"`
	From string `json:"from"`
	To   string `json:"to"`
}

// DeployDepositWallet 通过 Polymarket relayer 部署当前 EOA 对应的 DepositWallet。
//
// 仅 DepositWalletSignatureType 客户端可用。返回 relayer 已确认的链上 receipt。
//
// 部署是幂等的：如果链上已经部署，本方法会直接返回 nil（无 receipt），
// 避免重复 POST 浪费配额。要强制走 relayer 一次（比如调试），传 force=true。
//
// 注意：DepositWallet 的 ID 字段约定为 bytes32(uint160(EOA))；这是
// Polymarket 后端 operator 自己选的 salt，本地不参与。
func (c *GaslessClient) DeployDepositWallet(force bool) (*types.TransactionReceipt, error) {
	return c.DeployDepositWalletContext(context.Background(), force)
}

func (c *GaslessClient) DeployDepositWalletContext(ctx context.Context, force bool) (*types.TransactionReceipt, error) {
	if c.GetSignatureType() != types.DepositWalletSignatureType {
		return nil, fmt.Errorf("DeployDepositWallet 只支持 DepositWalletSignatureType/POLY_1271 (=3)，当前=%d", c.GetSignatureType())
	}

	walletHex, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("derive DepositWallet address: %w", err)
	}
	wallet := common.HexToAddress(string(walletHex))

	if !force {
		deployed, err := c.BaseClient.IsDepositWalletDeployed(ctx, wallet)
		if err != nil {
			return nil, fmt.Errorf("check deploy status: %w", err)
		}
		if deployed {
			log.Printf("[INFO] DepositWallet %s 已部署，跳过 deploy", wallet.Hex())
			return nil, nil
		}
	}

	body := &CWIADeployBody{
		Type: "WALLET-CREATE",
		From: string(c.GetBaseAddress()),
		To:   internal.PolygonDepositWalletFactory,
	}
	return c.submitGaslessSimpleContext(ctx, body, "WALLET-CREATE")
}

// submitGaslessSimple 提交一个简单的 relayer body（不像 batch 那样有 proxyTxns
// 数量等参数），与 executeGaslessBatch 共享同一套 HMAC + POST + receipt 等待逻辑。
//
// 这里没复用 executeGaslessBatch 是因为它的入口签名 (proxyTxns []map) 不适合
// 单笔无 calls 的请求；后续若需要更多 relayer 类型可以再统一抽。
func (c *GaslessClient) submitGaslessSimple(body interface{}, label string) (*types.TransactionReceipt, error) {
	return c.submitGaslessSimpleContext(context.Background(), body, label)
}

func (c *GaslessClient) submitGaslessSimpleContext(ctx context.Context, body interface{}, label string) (*types.TransactionReceipt, error) {
	bodyJSON, err := formatJSONWithSpaces(body)
	if err != nil {
		return nil, fmt.Errorf("format body: %w", err)
	}
	callCount := atomic.AddInt64(&c.relayerCallCount, 1)
	log.Printf("[Relayer调用 #%d] 提交 %s 到 relayer，body_bytes=%d", callCount, label, len(bodyJSON))

	headers, err := c.localSigner.SignRequest("POST", "/submit", body)
	if err != nil {
		return nil, fmt.Errorf("sign request: %w", err)
	}

	url := fmt.Sprintf("%s/submit", c.relayURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &sdkerrors.AmbiguousOutcomeError{Service: "relayer", Method: "POST", Path: "/submit", Operation: label, Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		errMsg := sdkhttp.SanitizeErrorResponse(bodyBytes, 200)
		apiErr := sdkhttp.APIErrorFromResponse("relayer", "POST", "/submit", resp, bodyBytes)
		return nil, &sdkerrors.RelayHTTPError{Status: resp.StatusCode, Body: errMsg, API: apiErr}
	}

	var gaslessResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&gaslessResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if state, ok := gaslessResp["state"].(string); ok &&
		(state == "STATE_FAILED" || state == "FAILED" || state == "failed") {
		txID, _ := gaslessResp["transactionID"].(string)
		errMsg, _ := gaslessResp["error"].(string)
		if errMsg == "" {
			if m, _ := gaslessResp["message"].(string); m != "" {
				errMsg = m
			}
		}
		log.Printf("[ERROR] [Relayer调用 #%d] %s 失败 (state=%s, txID=%s): %s",
			callCount, label, state, txID, errMsg)
		return nil, &sdkerrors.RelayFailedError{
			State: state, TransactionID: txID, Message: errMsg,
		}
	}

	txHashStr, _ := gaslessResp["transactionHash"].(string)
	if txHashStr == "" {
		if h, _ := gaslessResp["txHash"].(string); h != "" {
			txHashStr = h
		} else if h, _ := gaslessResp["hash"].(string); h != "" {
			txHashStr = h
		}
	}
	if txHashStr == "" {
		// relayer 异步部署：返回 transactionID 但还没上链 hash。
		// 调用方需要 poll /transaction 才能拿到最终 hash。
		txID, _ := gaslessResp["transactionID"].(string)
		state, _ := gaslessResp["state"].(string)
		log.Printf("[INFO] [Relayer调用 #%d] %s 已入队 (txID=%s, state=%s)，未返回交易 hash",
			callCount, label, txID, state)
		if txID == "" {
			return nil, fmt.Errorf("WALLET-CREATE returned neither transactionHash nor transactionID")
		}
		if err := c.waitForRelayerConfirmedContext(ctx, txID); err != nil {
			return nil, fmt.Errorf("wait WALLET-CREATE confirmation: %w", err)
		}
		return nil, nil
	}

	log.Printf("[OK] [Relayer调用 #%d] %s 成功，txHash=%s", callCount, label, txHashStr)
	receipt, err := c.waitForTransactionReceiptContext(ctx, common.HexToHash(txHashStr))
	if err != nil {
		return nil, fmt.Errorf("wait for receipt: %w", err)
	}
	if txID, _ := gaslessResp["transactionID"].(string); txID != "" {
		if err := c.waitForRelayerConfirmedContext(ctx, txID); err != nil {
			return nil, fmt.Errorf("wait WALLET-CREATE confirmation: %w", err)
		}
	}
	return receipt, nil
}
