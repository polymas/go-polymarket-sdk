package relayer

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// Polymarket relayer 用 type="WALLET" 表示 DepositWallet。已对照
// py-builder-relayer-client（github.com/Polymarket/py-builder-relayer-client）
// 中的 DepositWalletBatchRequest.to_dict() 校准 schema。
const cwiaRelayerWalletType = "WALLET"

// CWIADepositWalletParams 是 /submit body 中嵌套的 depositWalletParams。
//
// 字段顺序与官方 Python 客户端的 dict 插入顺序一致——这关系到 HMAC 签名所
// 用到的 body 字符串，不能乱序。
type CWIADepositWalletParams struct {
	DepositWallet string                 `json:"depositWallet"` // wallet 代理地址
	Deadline      string                 `json:"deadline"`      // unix 秒
	Calls         []CWIARelayCallPayload `json:"calls"`
}

// CWIARelayCallPayload 是单条 call 在 relayer body 中的 JSON 形态。
type CWIARelayCallPayload struct {
	Target string `json:"target"`
	Value  string `json:"value"` // 整数字符串
	Data   string `json:"data"`  // 0x-prefixed hex
}

// CWIARelayBody 与官方 DepositWalletBatchRequest.to_dict() 完全对齐：
//
//	{
//	  "type":      "WALLET",
//	  "from":      <EOA>,
//	  "to":        <factory>,
//	  "nonce":     <on-chain wallet nonce>,
//	  "signature": <EIP-712 batch sig 0x...>,
//	  "depositWalletParams": {
//	    "depositWallet": <wallet>,
//	    "deadline":      <unix>,
//	    "calls":         [{target, value, data}, ...]
//	  }
//	}
type CWIARelayBody struct {
	Type                string                  `json:"type"`
	From                string                  `json:"from"`
	To                  string                  `json:"to"`
	Nonce               string                  `json:"nonce"`
	Signature           string                  `json:"signature"`
	DepositWalletParams CWIADepositWalletParams `json:"depositWalletParams"`
}

// buildCWIARelayTransactionBatch 把 GaslessClient 的 proxyTxns ([]map) 转换为
// CWIA Batch、签名，并打包成提交到 relayer 的请求体。
//
// proxyTxns 中每条期望包含 {"to": <addr>, "data": <0x...>, "value": <int>}
// （typeCode/operation 字段会被忽略——CWIA 一律 Call，没有 DelegateCall）。
//
// 与 PROXY/SAFE 关键区别：CWIA 的 body 不含 `data`（由 relayer 自行编码
// `factory.proxy(Batch[],bytes[])` 调用），用户只提供原始 calls + signature。
func (c *GaslessClient) buildCWIARelayTransactionBatch(
	proxyTxns []map[string]interface{},
	_ string, // metadata：CWIA 端没有 metadata 字段，忽略
) (*CWIARelayBody, error) {
	return c.buildCWIARelayTransactionBatchContext(context.Background(), proxyTxns, "")
}

func (c *GaslessClient) buildCWIARelayTransactionBatchContext(ctx context.Context, proxyTxns []map[string]interface{}, _ string) (*CWIARelayBody, error) {
	if len(proxyTxns) == 0 {
		return nil, fmt.Errorf("no transactions to batch")
	}

	walletHex, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("get DepositWallet proxy address: %w", err)
	}
	wallet := common.HexToAddress(string(walletHex))

	calls := make([]web3.CWIACallInput, 0, len(proxyTxns))
	for i, t := range proxyTxns {
		toStr, _ := t["to"].(string)
		if toStr == "" {
			return nil, fmt.Errorf("proxyTxns[%d].to missing or not string", i)
		}
		dataHex, _ := t["data"].(string)
		dataBytes, derr := hex.DecodeString(strings.TrimPrefix(dataHex, "0x"))
		if derr != nil {
			return nil, fmt.Errorf("proxyTxns[%d].data hex decode: %w", i, derr)
		}
		v := big.NewInt(0)
		if vi, ok := t["value"].(int); ok {
			v = big.NewInt(int64(vi))
		} else if vbi, ok := t["value"].(*big.Int); ok && vbi != nil {
			v = vbi
		}
		calls = append(calls, web3.CWIACallInput{
			To:    common.HexToAddress(toStr),
			Value: v,
			Data:  dataBytes,
		})
	}

	// V2 协议下 nonce 必须用 relayer 的 GET /nonce?address=EOA&type=WALLET，
	// 不能用 wallet.nonce() 链上读 — relayer 维护了自己的 nonce 计数器，包括
	// 已提交未上链的 pending tx；用链上 nonce 会被 relayer 校验失败回 500。
	// 这是 polymarket/builder-relayer-client 官方 executeDepositWalletBatch 的做法。
	relayerNonce, err := c.getRelayNonceContext(ctx, "WALLET")
	if err != nil {
		return nil, fmt.Errorf("get WALLET nonce from relayer: %w", err)
	}
	nonceBig := big.NewInt(int64(relayerNonce))

	batch, sig, _, err := c.BaseClient.BuildAndSignCWIABatch(
		ctx, wallet, calls, nonceBig, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build+sign CWIA batch: %w", err)
	}

	factoryAddr := common.HexToAddress(internal.PolygonDepositWalletFactory)

	relayCalls := make([]CWIARelayCallPayload, len(batch.Calls))
	for i, call := range batch.Calls {
		relayCalls[i] = CWIARelayCallPayload{
			Target: call.Target.Hex(),
			Value:  call.Value.String(),
			Data:   "0x" + hex.EncodeToString(call.Data),
		}
	}

	return &CWIARelayBody{
		Type:      cwiaRelayerWalletType,
		From:      string(c.GetBaseAddress()),
		To:        factoryAddr.Hex(),
		Nonce:     batch.Nonce.String(),
		Signature: "0x" + hex.EncodeToString(sig),
		DepositWalletParams: CWIADepositWalletParams{
			DepositWallet: wallet.Hex(),
			Deadline:      batch.Deadline.String(),
			Calls:         relayCalls,
		},
	}, nil
}
