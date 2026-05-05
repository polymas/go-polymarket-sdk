package web3

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

// 本文件提供 CWIA / DepositWallet 链上批量执行所需的全部确定性组件，
// 不依赖 Polymarket relayer：
//
//   - GetDepositWalletNonce  : 读取 wallet.nonce()（链上 ground truth）
//   - BuildCWIABatchCalldata : 把单条 (batch, signature) 编码成
//                              factory.proxy([batch], [sig]) 的 calldata，
//                              可由任何持有 operator 角色的地址直接广播
//
// gasless 提交（走 Polymarket relayer）的 body 字段名 / 鉴权头复用现有
// /submit + HMAC 流程，但 CWIA 这套类型 (`type=WALLET`) 在公开渠道未见
// schema 文档，所以 ExecuteCWIAGaslessBatch 的请求体目前按照与 PROXY/SAFE
// 同构的最简映射构造（见函数注释），上线前需按真实抓包再校准。

// CWIACallInput 是构造 Batch 时用户提供的单条 call。
type CWIACallInput struct {
	To    common.Address
	Value *big.Int // 通常为 0，nil 视作 0
	Data  []byte
}

// GetDepositWalletNonce 读取给定 DepositWallet 的链上 nonce。
//
// 这是 Batch.nonce 字段的唯一可信来源 —— relayer /nonce 接口在迁移期
// 可能滞后或缓存（参考 Safe 的 GS025 教训）。
func (c *baseClient) GetDepositWalletNonce(ctx context.Context, wallet common.Address) (uint64, error) {
	// nonce() selector = 0xaffed0e0
	data, err := hex.DecodeString("affed0e0")
	if err != nil {
		return 0, err
	}
	res, err := c.callContractWithRetry(ctx, ethereum.CallMsg{To: &wallet, Data: data}, nil)
	if err != nil {
		return 0, fmt.Errorf("read wallet.nonce(): %w", err)
	}
	if len(res) == 0 {
		return 0, fmt.Errorf("empty result from wallet.nonce()")
	}
	return new(big.Int).SetBytes(res).Uint64(), nil
}

// BuildCWIABatchCalldata 编码 DepositWalletFactory.proxy([batch], [sig]) 调用。
//
// 选择子：keccak256("proxy((address,uint256,uint256,(address,uint256,bytes)[])[],bytes[])")[:4]
// = 0x0a3c4405（已对照 Polygon 真实 tx 验证）。
//
// 返回的 calldata 直接以 operator 身份发往 PolygonDepositWalletFactory 即可上链。
func BuildCWIABatchCalldata(batch signing.CWIABatch, signature []byte) ([]byte, error) {
	parsed, err := getDepositWalletFactoryProxyABI()
	if err != nil {
		return nil, err
	}

	// 转换 batch 到 ABI 期望的 tuple 形式
	type onchainCall struct {
		Target common.Address `abi:"target"`
		Value  *big.Int       `abi:"value"`
		Data   []byte         `abi:"data"`
	}
	type onchainBatch struct {
		Wallet   common.Address `abi:"wallet"`
		Nonce    *big.Int       `abi:"nonce"`
		Deadline *big.Int       `abi:"deadline"`
		Calls    []onchainCall  `abi:"calls"`
	}

	calls := make([]onchainCall, len(batch.Calls))
	for i, c := range batch.Calls {
		v := c.Value
		if v == nil {
			v = big.NewInt(0)
		}
		calls[i] = onchainCall{Target: c.Target, Value: v, Data: c.Data}
	}
	ob := onchainBatch{
		Wallet:   batch.Wallet,
		Nonce:    batch.Nonce,
		Deadline: batch.Deadline,
		Calls:    calls,
	}

	return parsed.Pack("proxy", []onchainBatch{ob}, [][]byte{signature})
}

// BuildAndSignCWIABatch 是把 (proxy 地址, calls, deadline) 一站式封装为
// (batch, signature, factoryCalldata) 的便捷函数。
//
// 入参 nonce==0 时自动从链上读取当前 wallet.nonce()。
// deadline==0 时默认设置为当前时间 + 10 分钟。
func (c *baseClient) BuildAndSignCWIABatch(
	ctx context.Context,
	wallet common.Address,
	calls []CWIACallInput,
	nonce *big.Int,
	deadline *big.Int,
) (signing.CWIABatch, []byte, []byte, error) {
	if c.signatureType != types.CWIASignatureType {
		return signing.CWIABatch{}, nil, nil, fmt.Errorf(
			"BuildAndSignCWIABatch requires CWIASignatureType client, got %d", c.signatureType)
	}
	if len(calls) == 0 {
		return signing.CWIABatch{}, nil, nil, fmt.Errorf("calls must not be empty")
	}

	if nonce == nil || nonce.Sign() == 0 {
		n, err := c.GetDepositWalletNonce(ctx, wallet)
		if err != nil {
			return signing.CWIABatch{}, nil, nil, err
		}
		nonce = new(big.Int).SetUint64(n)
	}
	if deadline == nil || deadline.Sign() == 0 {
		deadline = big.NewInt(time.Now().Add(10 * time.Minute).Unix())
	}

	bcalls := make([]signing.CWIACall, len(calls))
	for i, c := range calls {
		v := c.Value
		if v == nil {
			v = big.NewInt(0)
		}
		bcalls[i] = signing.CWIACall{Target: c.To, Value: v, Data: c.Data}
	}
	batch := signing.CWIABatch{
		Wallet:   wallet,
		Nonce:    nonce,
		Deadline: deadline,
		Calls:    bcalls,
	}

	chainID := big.NewInt(int64(c.chainID))
	sig, err := signing.SignCWIABatch(c.privateKey, chainID, batch)
	if err != nil {
		return signing.CWIABatch{}, nil, nil, err
	}

	calldata, err := BuildCWIABatchCalldata(batch, sig)
	if err != nil {
		return signing.CWIABatch{}, nil, nil, err
	}
	return batch, sig, calldata, nil
}

// getDepositWalletFactoryProxyABI 返回 factory.proxy(Batch[], bytes[]) 的最小 ABI。
func getDepositWalletFactoryProxyABI() (*abi.ABI, error) {
	abiJSON := `[{
		"inputs": [
			{
				"components": [
					{"name": "wallet",   "type": "address"},
					{"name": "nonce",    "type": "uint256"},
					{"name": "deadline", "type": "uint256"},
					{
						"components": [
							{"name": "target", "type": "address"},
							{"name": "value",  "type": "uint256"},
							{"name": "data",   "type": "bytes"}
						],
						"name": "calls",
						"type": "tuple[]"
					}
				],
				"name": "_batches",
				"type": "tuple[]"
			},
			{"name": "_signatures", "type": "bytes[]"}
		],
		"name": "proxy",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, fmt.Errorf("parse factory.proxy ABI: %w", err)
	}
	return &parsed, nil
}

// Polymarket relayer 对 CWIA 用 type="WALLET"（已验证 /nonce 接受此值并返回
// 与链上 wallet.nonce() 一致的数值）。
const cwiaRelayerWalletType = "WALLET"

// CWIARelayBody 是 GaslessClient 提交到 /submit 的 CWIA 请求体。
//
// ⚠️ 注意：Polymarket 公开渠道未发布该 type 的 schema。本结构基于以下推断：
//  1. /nonce?type=WALLET 已验证存在并返回 wallet.nonce()
//  2. 字段命名沿用 PROXY/SAFE 同构风格
//  3. CWIA 的链上签名（Batch EIP-712）已嵌在 data 中（factory.proxy 调用
//     的第二个参数），所以 signature 这里留空（与 PROXY/SAFE 不同）
//
// 如果首次提交被 relayer 拒绝，错误响应会指明缺失/多余字段，按需调整即可。
type CWIARelayBody struct {
	Data        string `json:"data"`        // factory.proxy([batch],[sig]) calldata（含 0x）
	From        string `json:"from"`        // 签名者 EOA
	Metadata    string `json:"metadata"`    //
	Nonce       string `json:"nonce"`       // 链上 wallet.nonce() 字符串
	ProxyWallet string `json:"proxyWallet"` // DepositWallet 代理地址
	Signature   string `json:"signature"`   // 留空（batch sig 已在 data 内）
	To          string `json:"to"`          // PolygonDepositWalletFactory
	Type        string `json:"type"`        // "WALLET"
}

// buildCWIARelayTransactionBatch 把 GaslessClient 的 proxyTxns ([]map) 转换为
// CWIA Batch、签名，并打包成提交到 relayer 的请求体。
//
// proxyTxns 中每条期望包含至少 {"to": <addr>, "data": <0x...>, "value": <int>}
// （typeCode/operation 字段会被忽略）。
func (c *GaslessClient) buildCWIARelayTransactionBatch(
	proxyTxns []map[string]interface{},
	metadata string,
) (*CWIARelayBody, error) {
	if len(proxyTxns) == 0 {
		return nil, fmt.Errorf("no transactions to batch")
	}

	walletHex, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("get DepositWallet proxy address: %w", err)
	}
	wallet := common.HexToAddress(string(walletHex))

	calls := make([]CWIACallInput, 0, len(proxyTxns))
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
		calls = append(calls, CWIACallInput{
			To:    common.HexToAddress(toStr),
			Value: v,
			Data:  dataBytes,
		})
	}

	batch, sig, calldata, err := c.baseClient.BuildAndSignCWIABatch(
		context.Background(), wallet, calls, nil, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build+sign CWIA batch: %w", err)
	}

	factoryAddr := common.HexToAddress(internal.PolygonDepositWalletFactory)

	return &CWIARelayBody{
		Data:        "0x" + hex.EncodeToString(calldata),
		From:        string(c.baseAddress),
		Metadata:    metadata,
		Nonce:       batch.Nonce.String(),
		ProxyWallet: wallet.Hex(),
		Signature:   "0x" + hex.EncodeToString(sig), // 同时带上 batch sig，relayer 不用就忽略
		To:          factoryAddr.Hex(),
		Type:        cwiaRelayerWalletType,
	}, nil
}
