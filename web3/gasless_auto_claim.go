package web3

import (
	"context"
	"fmt"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// Polymarket 的 auto-claim 就是给 `PolymarketAutoClaimer` 合约在 ConditionalTokens 上授予
// ERC1155 operator 权限。授权后 Polymarket 后端在市场结算后自动帮用户 redeem 赢家仓位，
// collateral 以 pUSD 形式直接回到用户 Safe；关闭就是把 approved 翻回 false。
//
// 这个功能在官方文档（含 llms-full.txt）里没有任何文字，是纯链上行为，
// 从 tx 0x22d6dcec5feb6bce8fdec26b86e1ee1b54aa333819aabc19d70f3006680cfd55 反推出来的。
//
// 一次 setApprovalForAll 同时覆盖 Regular 和 NegRisk 市场，因为两类 position token
// 都归同一个 ConditionalTokens 合约持有。

const isApprovedForAllABISource = `[{"name":"isApprovedForAll","type":"function","stateMutability":"view","inputs":[{"name":"account","type":"address"},{"name":"operator","type":"address"}],"outputs":[{"type":"bool"}]}]`

var isApprovedForAllAB abi.ABI

func init() {
	parsed, err := abi.JSON(strings.NewReader(isApprovedForAllABISource))
	if err != nil {
		panic(fmt.Sprintf("parse isApprovedForAllABI: %v", err))
	}
	isApprovedForAllAB = parsed
}

// SetAutoClaim 打开（enabled=true）或关闭（enabled=false）Polymarket 的自动 redeem 服务。
// 内部就是一笔 CTF.setApprovalForAll(PolymarketAutoClaimer, enabled)，走用户 Safe 由
// Relayer 免 gas 提交。
//
// 开启：Polymarket 后端在市场结算后自动把赢家仓位换成 pUSD 送回 Safe，用户不用手动 redeem。
// 关闭：立即撤销 operator 权限，回到手动 redeem。之前已发送的自动 redeem tx 不受影响。
func (c *GaslessClient) SetAutoClaim(enabled bool) (*types.TransactionReceipt, error) {
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	claimer := common.HexToAddress(internal.PolymarketAutoClaimer)

	data, err := packSetApprovalForAllWithFlag(claimer, enabled)
	if err != nil {
		return nil, fmt.Errorf("pack setApprovalForAll auto-claimer: %w", err)
	}

	label := "Enable Polymarket auto-claim"
	tag := "auto-claim-on"
	if !enabled {
		label = "Disable Polymarket auto-claim"
		tag = "auto-claim-off"
	}
	return c.executeGaslessBatch([]map[string]any{callTxn(ctf, data)}, label, tag)
}

// IsAutoClaimEnabled 只读查询当前 Safe 对 PolymarketAutoClaimer 的 operator 授权状态。
// 走 eth_call，不发链上交易。
func (c *GaslessClient) IsAutoClaimEnabled() (bool, error) {
	safeStr, err := c.GetPolyProxyAddress()
	if err != nil {
		return false, fmt.Errorf("GetPolyProxyAddress: %w", err)
	}
	owner := common.HexToAddress(string(safeStr))
	claimer := common.HexToAddress(internal.PolymarketAutoClaimer)
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)

	calldata, err := isApprovedForAllAB.Pack("isApprovedForAll", owner, claimer)
	if err != nil {
		return false, fmt.Errorf("pack isApprovedForAll: %w", err)
	}
	out, err := c.callContractWithRetry(context.Background(), ethereum.CallMsg{To: &ctf, Data: calldata}, nil)
	if err != nil {
		return false, fmt.Errorf("eth_call isApprovedForAll: %w", err)
	}
	var approved bool
	if err := isApprovedForAllAB.UnpackIntoInterface(&approved, "isApprovedForAll", out); err != nil {
		return false, fmt.Errorf("unpack isApprovedForAll: %w", err)
	}
	return approved, nil
}

// packSetApprovalForAllWithFlag 复用 setApprovalForAllAB，把 approved 作为可变参数
// （gasless_approvals.go 里的 packSetApprovalForAll 硬编码 true，仅用于授权场景）。
func packSetApprovalForAllWithFlag(operator common.Address, approved bool) ([]byte, error) {
	return setApprovalForAllAB.Pack("setApprovalForAll", operator, approved)
}

// EnableAutoClaim 幂等地开启 auto-claim：先读链上状态，已开则跳过链上写直接返回
// (nil, nil)；未开则调用 SetAutoClaim(true) 发一笔 gasless tx。
// 适合放进钱包初始化 / 用户首次进场的"确保 X 已就位"流程，重复调用零成本。
func (c *GaslessClient) EnableAutoClaim() (*types.TransactionReceipt, error) {
	on, err := c.IsAutoClaimEnabled()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if on {
		return nil, nil
	}
	return c.SetAutoClaim(true)
}

// DisableAutoClaim 幂等地关闭 auto-claim，与 EnableAutoClaim 对称。
// 已关则直接返回 (nil, nil)。
func (c *GaslessClient) DisableAutoClaim() (*types.TransactionReceipt, error) {
	on, err := c.IsAutoClaimEnabled()
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if !on {
		return nil, nil
	}
	return c.SetAutoClaim(false)
}
