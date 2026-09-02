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
// gasless 提交（走 Polymarket relayer，type=WALLET）的请求体构造已移到
// web3/relayer 包（relayer/cwia_gasless.go）。

// CWIACallInput 是构造 Batch 时用户提供的单条 call。
type CWIACallInput struct {
	To    common.Address
	Value *big.Int // 通常为 0，nil 视作 0
	Data  []byte
}

// ErrDepositWalletNotDeployed 表示 DepositWallet 代理地址被推导出来但合约
// 还没部署到链上。新注册的 CWIA 用户首次发批量交易前必须先调
// relayer.GaslessClient.DeployDepositWallet() 走 type=WALLET-CREATE 在 relayer
// 部署钱包。
var ErrDepositWalletNotDeployed = fmt.Errorf("DepositWallet not deployed")

// GetDepositWalletNonce 读取给定 DepositWallet 的链上 nonce。
//
// 这是 Batch.nonce 字段的唯一可信来源 —— relayer /nonce 接口在迁移期
// 可能滞后或缓存（参考 Safe 的 GS025 教训）。
//
// 如果 DepositWallet 还未部署（counter-factual 地址但链上无合约代码），
// eth_call wallet.nonce() 返回空 bytes —— 此时返回 ErrDepositWalletNotDeployed。
// 调用方应先通过 relayer.GaslessClient.DeployDepositWallet() 部署。
func (c *BaseClient) GetDepositWalletNonce(ctx context.Context, wallet common.Address) (uint64, error) {
	// nonce() selector = 0xaffed0e0
	data, err := hex.DecodeString("affed0e0")
	if err != nil {
		return 0, err
	}
	res, err := c.CallContractWithRetry(ctx, ethereum.CallMsg{To: &wallet, Data: data}, nil)
	if err != nil {
		return 0, fmt.Errorf("read wallet.nonce(): %w", err)
	}
	if len(res) == 0 {
		return 0, fmt.Errorf("%w (%s)：先调用 relayer.GaslessClient.DeployDepositWallet() 部署",
			ErrDepositWalletNotDeployed, wallet.Hex())
	}
	return new(big.Int).SetBytes(res).Uint64(), nil
}

// IsDepositWalletDeployed 探测给定地址上是否有合约代码（DepositWallet 是否已部署）。
// 不区分 ERC-7760 / 其他，只检查"地址有 code"。便于业务层在调用 batch 前
// 决定是否要先 deploy。
func (c *BaseClient) IsDepositWalletDeployed(ctx context.Context, wallet common.Address) (bool, error) {
	var (
		code []byte
		err  error
	)
	if c.depositWalletRPC != nil && c.depositWalletRPC.code != nil {
		code, err = c.depositWalletRPC.code(ctx, wallet)
	} else {
		code, err = c.CodeAtWithRetry(ctx, wallet)
	}
	if err != nil {
		return false, fmt.Errorf("probe wallet code: %w", err)
	}
	return len(code) > 0, nil
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
func (c *BaseClient) BuildAndSignCWIABatch(
	ctx context.Context,
	wallet common.Address,
	calls []CWIACallInput,
	nonce *big.Int,
	deadline *big.Int,
) (signing.CWIABatch, []byte, []byte, error) {
	if c.signatureType != types.DepositWalletSignatureType {
		return signing.CWIABatch{}, nil, nil, fmt.Errorf(
			"BuildAndSignCWIABatch requires DepositWalletSignatureType/POLY_1271 client, got %d", c.signatureType)
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
	sig, err := signing.SignCWIABatchWithSigner(c.signer, chainID, batch)
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
