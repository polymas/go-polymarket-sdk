package relayer

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// erc20TransferABISource 只用到 transfer（balanceOf 已在 erc20ABI 里覆盖）。
const erc20TransferABISource = `[{"name":"transfer","type":"function","stateMutability":"nonpayable","inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"type":"bool"}]}]`

var erc20TransferABI abi.ABI

func init() {
	parsed, err := abi.JSON(strings.NewReader(erc20TransferABISource))
	if err != nil {
		panic(fmt.Sprintf("parse erc20 transfer ABI: %v", err))
	}
	erc20TransferABI = parsed
}

// TransferERC20 让 proxy/safe 钱包通过 relayer 调 ERC20.transfer(to, amountUnits)。
// amountUnits 已是该代币的最小单位（不再做小数换算）。
func (c *GaslessClient) TransferERC20(token, to common.Address, amountUnits *big.Int) (*types.TransactionReceipt, error) {
	if amountUnits == nil || amountUnits.Sign() <= 0 {
		return nil, fmt.Errorf("amountUnits 必须 > 0")
	}
	data, err := erc20TransferABI.Pack("transfer", to, amountUnits)
	if err != nil {
		return nil, fmt.Errorf("pack transfer: %w", err)
	}
	txn := map[string]any{
		"typeCode": 1,
		"to":       token.Hex(),
		"value":    0,
		"data":     "0x" + hex.EncodeToString(data),
	}
	return c.executeGaslessBatch([]map[string]any{txn}, fmt.Sprintf("ERC20 transfer %s → %s", token.Hex(), to.Hex()), "erc20-transfer")
}

// WithdrawPUSD 从 proxy 把 amount 个 pUSD（人类单位，6 decimals）转给 to。
// amount<=0 时返回错误。
//
// 注意：pUSD 是 Polymarket 内部受限代币，直接 ERC20.transfer 到非白名单地址会 revert。
// 真实"提现"应使用 UnwrapPUSDToUSDC，把 pUSD 烧掉、把等额 USDC.e 直接发给收款方。
func (c *GaslessClient) WithdrawPUSD(to common.Address, amount float64) (*types.TransactionReceipt, error) {
	units, err := toUnits6(amount)
	if err != nil {
		return nil, fmt.Errorf("amount: %w", err)
	}
	return c.TransferERC20(common.HexToAddress(internal.PolygonPUSD), to, units)
}

// collateralOfframpABI: pUSD → USDC.e 的 unwrap 合约接口。
const collateralOfframpABISource = `[{"name":"unwrap","type":"function","stateMutability":"nonpayable","inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[]}]`

var collateralOfframpABI abi.ABI

func init() {
	parsed, err := abi.JSON(strings.NewReader(collateralOfframpABISource))
	if err != nil {
		panic(fmt.Sprintf("parse offramp ABI: %v", err))
	}
	collateralOfframpABI = parsed
}

// UnwrapPUSDToUSDC 用 proxy 调 CollateralOfframp.unwrap(to, amount)：烧掉 proxy 持有的
// pUSD，把等额 USDC.e 直接发到 to。amount 是人类单位（1.0 = 1 pUSD = 1 USDC.e，6 decimals）。
// 这是从 Polymarket 钱包提现 USDC.e 到任意地址的正路。
func (c *GaslessClient) UnwrapPUSDToUSDC(to common.Address, amount float64) (*types.TransactionReceipt, error) {
	units, err := toUnits6(amount)
	if err != nil {
		return nil, fmt.Errorf("amount: %w", err)
	}
	data, err := collateralOfframpABI.Pack("unwrap", to, units)
	if err != nil {
		return nil, fmt.Errorf("pack unwrap: %w", err)
	}
	txn := callTxn(common.HexToAddress(internal.PolygonCollateralOfframp), data)
	return c.executeGaslessBatch([]map[string]any{txn},
		fmt.Sprintf("Offramp.unwrap %.6f pUSD → %s", amount, to.Hex()),
		"withdraw-unwrap")
}

// GetERC20BalanceOf 用 RPC 调 token.balanceOf(owner)，返回最小单位余额。
// 与同包的无参 GetPUSDBalance() 互补：那个只查 proxy 自己；这个可查任意地址、任意 ERC20。
func (c *GaslessClient) GetERC20BalanceOf(token, owner common.Address) (*big.Int, error) {
	return c.callERC20Uint(context.Background(), token, "balanceOf", owner)
}
