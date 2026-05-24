package web3

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// EnsureV2Ready 业务层进场前的"一键自检+自动修复"。
// 全程幂等：
//  1. Safe 上有 USDC.e → 自动 wrap 成 pUSD（顺手补 USDC.e→CollateralOnramp 的 approve 如果没到位）
//     Safe 上没 USDC.e → 跳过 wrap
//  2. pUSD 对 V2 trading 必备 spender（V2 Exchange / V2 NegRisk Exchange / NegRiskAdapter）
//     每个未授权的 → 补 approve(MAX)
//  3. CTF 对 V2 trading 必备 operator（V2 Exchange / V2 NegRisk Exchange / NegRiskAdapter）
//     每个未 isApprovedForAll → setApprovalForAll(true)
//
// 注意：CtfCollateralAdapter / NegRiskCtfCollateralAdapter 这两个 adapter 不在 Polymarket
// relayer 的 builder 白名单里，approve / setApprovalForAll 它们的 calldata 会被 relayer 直接
// 拒（"call blocked: approve spender ... is not in the allowed list"），所以这里**不**把它们
// 算进必查项。需要 split/merge 走 adapter 路径的业务必须通过其它通道（EOA 直签 / Safe 多签等）
// 完成这两个 adapter 的授权，IsV2Ready 不再代为兜底，避免 EnsureV2Ready 无限重试。
//
// 全部已就位 → 返回 (nil, nil)，**不发任何 tx**。
// 否则把所有需要的操作打包成**一笔 gasless batch** 发出去，返回 receipt。
func (c *GaslessClient) EnsureV2Ready() (*types.TransactionReceipt, error) {
	ctx := context.Background()
	safeStr, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, fmt.Errorf("GetPolyProxyAddress: %w", err)
	}
	safe := common.HexToAddress(string(safeStr))

	state, err := c.readV2State(ctx, safe)
	if err != nil {
		return nil, err
	}

	proxyTxns, err := c.buildEnsureBatch(safe, state)
	if err != nil {
		return nil, err
	}
	if len(proxyTxns) == 0 {
		return nil, nil
	}
	return c.executeGaslessBatch(proxyTxns, "Ensure V2 Ready", "ensure-v2")
}

// EnsureV2ReadyOneByOne 把 buildEnsureBatch 生成的每一笔 proxy tx 各自单独提交一次，
// 用于诊断 "组合 batch 整体被 relayer 吞掉但单笔可能能过" 的场景。
// 返回值：每笔的 (receipt, err)，按 buildEnsureBatch 的顺序排列。
//
// 没有任何 missing 项时返回 (nil, nil, nil)；中途某笔失败也继续提交后续，把所有结果返回让调用方看。
func (c *GaslessClient) EnsureV2ReadyOneByOne() (receipts []*types.TransactionReceipt, errs []error, err error) {
	ctx := context.Background()
	safeStr, err := c.GetPolyProxyAddress()
	if err != nil {
		return nil, nil, fmt.Errorf("GetPolyProxyAddress: %w", err)
	}
	safe := common.HexToAddress(string(safeStr))

	state, err := c.readV2State(ctx, safe)
	if err != nil {
		return nil, nil, err
	}

	proxyTxns, err := c.buildEnsureBatch(safe, state)
	if err != nil {
		return nil, nil, err
	}
	if len(proxyTxns) == 0 {
		return nil, nil, nil
	}

	receipts = make([]*types.TransactionReceipt, len(proxyTxns))
	errs = make([]error, len(proxyTxns))
	for i, tx := range proxyTxns {
		r, e := c.executeGaslessBatch([]map[string]interface{}{tx},
			fmt.Sprintf("Ensure V2 (split %d/%d)", i+1, len(proxyTxns)),
			fmt.Sprintf("ensure-v2-split-%d", i))
		receipts[i] = r
		errs[i] = e
	}
	return receipts, errs, nil
}

// IsV2Ready 只读版自检：返回 ready=true 说明 EnsureV2Ready() 当前是 no-op（无需发 tx），
// ready=false 时 missing 列出待修复项。**不发任何 tx**，仅链上 RPC 查询。
//
// 业务层用法：
//
//	ready, missing, err := gasless.IsV2Ready()
//	if !ready { /* 提示 / 自动调 EnsureV2Ready / 等等 */ }
func (c *GaslessClient) IsV2Ready() (ready bool, missing []string, err error) {
	ctx := context.Background()
	safeStr, err := c.GetPolyProxyAddress()
	if err != nil {
		return false, nil, fmt.Errorf("GetPolyProxyAddress: %w", err)
	}
	safe := common.HexToAddress(string(safeStr))
	state, err := c.readV2State(ctx, safe)
	if err != nil {
		return false, nil, err
	}
	missing = state.missing()
	return len(missing) == 0, missing, nil
}

// v2State 收纳 EnsureV2Ready 决策需要的链上状态快照。
type v2State struct {
	usdcBal     *big.Int                    // Safe 上 USDC.e 余额
	onrampAllow *big.Int                    // USDC.e → CollateralOnramp allowance
	pusdAllow   map[common.Address]*big.Int // pUSD → spender allowance
	ctfAppr     map[common.Address]bool     // CTF.isApprovedForAll(safe, operator)
}

// pusdSpendersV2 / ctfOperatorsV2 是 **可由 gasless relayer 兜底** 的 V2 trading
// spender / operator 列表 —— 即 relayer builder 白名单允许我们代发 approve / setApprovalForAll
// 的目标。出现顺序就是 missing[] 的展示顺序，调用方读起来稳定。
//
// 不在此处的（CtfCollateralAdapter / NegRiskCtfCollateralAdapter）见 EnsureV2Ready 顶注：
// relayer 拒绝代发，需走其它授权通道；IsV2Ready 不再把它们当作必修项。
func pusdSpendersV2() []common.Address {
	return []common.Address{
		common.HexToAddress(internal.PolygonExchangeV2),
		common.HexToAddress(internal.PolygonNegRiskExchangeV2),
		common.HexToAddress(internal.PolygonNegRiskAdapter),
	}
}

func ctfOperatorsV2() []common.Address {
	return []common.Address{
		common.HexToAddress(internal.PolygonExchangeV2),
		common.HexToAddress(internal.PolygonNegRiskExchangeV2),
		// NegRisk redeem 直调 NegRiskAdapter.redeemPositions(...) 时，adapter 内部
		// 会 ctf.safeBatchTransferFrom(safe → adapter)，因此 Safe 必须事先把
		// NegRiskAdapter 设为 ERC1155 operator。
		common.HexToAddress(internal.PolygonNegRiskAdapter),
	}
}

// readV2State 并发读所有需要的链上状态，单次失败即整体失败。
// 总共 8 笔 eth_call（1 USDC.e balance + 1 onramp allowance + 3 pUSD allowance + 3 CTF approval），
// 公网 RPC 串行约 2-5s，并发收敛到 ~300ms 一跳。
func (c *GaslessClient) readV2State(ctx context.Context, safe common.Address) (*v2State, error) {
	usdc := common.HexToAddress(internal.PolygonCollateral)
	pusd := common.HexToAddress(internal.PolygonPUSD)
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	onramp := common.HexToAddress(internal.PolygonCollateralOnramp)

	state := &v2State{
		pusdAllow: make(map[common.Address]*big.Int, 3),
		ctfAppr:   make(map[common.Address]bool, 3),
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex // 保护 state map / firstErr 共享写
		firstErr error
	)
	setErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		v, err := c.callERC20Uint(ctx, usdc, "balanceOf", safe)
		if err != nil {
			setErr(fmt.Errorf("USDC.e.balanceOf: %w", err))
			return
		}
		mu.Lock()
		state.usdcBal = v
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		v, err := c.callERC20Uint(ctx, usdc, "allowance", safe, onramp)
		if err != nil {
			setErr(fmt.Errorf("USDC.e allowance to Onramp: %w", err))
			return
		}
		mu.Lock()
		state.onrampAllow = v
		mu.Unlock()
	}()

	for _, sp := range pusdSpendersV2() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := c.callERC20Uint(ctx, pusd, "allowance", safe, sp)
			if err != nil {
				setErr(fmt.Errorf("pUSD allowance to %s: %w", sp.Hex(), err))
				return
			}
			mu.Lock()
			state.pusdAllow[sp] = v
			mu.Unlock()
		}()
	}

	for _, op := range ctfOperatorsV2() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			calldata, err := isApprovedForAllAB.Pack("isApprovedForAll", safe, op)
			if err != nil {
				setErr(fmt.Errorf("pack isApprovedForAll: %w", err))
				return
			}
			out, err := c.callContractWithRetry(ctx, ethereum.CallMsg{To: &ctf, Data: calldata}, nil)
			if err != nil {
				setErr(fmt.Errorf("CTF.isApprovedForAll(%s): %w", op.Hex(), err))
				return
			}
			var b bool
			if err := isApprovedForAllAB.UnpackIntoInterface(&b, "isApprovedForAll", out); err != nil {
				setErr(fmt.Errorf("unpack isApprovedForAll(%s): %w", op.Hex(), err))
				return
			}
			mu.Lock()
			state.ctfAppr[op] = b
			mu.Unlock()
		}()
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return state, nil
}

// GetPUSDBalance 读 Safe proxy 在 pUSD 合约上的余额，返回人类单位（1.0 = 1 pUSD，6 decimals）。
// 单笔 eth_call，不发 tx；与 clob.GetBalanceAllowance() 服务端版相比直接读链上、无缓存延迟。
//
// 业务层下单前的"我的钱够不够"判定建议优先用这个：服务端 balance/allowance 缓存有时滞后于链上，
// 而 EnsureV2Ready 刚 wrap 完时服务端还可能是旧值。
func (c *GaslessClient) GetPUSDBalance() (float64, error) {
	safeStr, err := c.GetPolyProxyAddress()
	if err != nil {
		return 0, fmt.Errorf("GetPolyProxyAddress: %w", err)
	}
	safe := common.HexToAddress(string(safeStr))
	raw, err := c.callERC20Uint(context.Background(), common.HexToAddress(internal.PolygonPUSD), "balanceOf", safe)
	if err != nil {
		return 0, fmt.Errorf("pUSD.balanceOf: %w", err)
	}
	f, _ := new(big.Float).Quo(new(big.Float).SetInt(raw), big.NewFloat(1e6)).Float64()
	return f, nil
}

// callERC20Uint 调 ERC20 view 函数（balanceOf / allowance 等返回 uint256）。
func (c *GaslessClient) callERC20Uint(ctx context.Context, token common.Address, method string, args ...any) (*big.Int, error) {
	calldata, err := erc20ABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", method, err)
	}
	out, err := c.callContractWithRetry(ctx, ethereum.CallMsg{To: &token, Data: calldata}, nil)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	var v *big.Int
	if err := erc20ABI.UnpackIntoInterface(&v, method, out); err != nil {
		return nil, fmt.Errorf("unpack %s: %w", method, err)
	}
	return v, nil
}

// missing 把缺失项格式化成人话列表，给 IsV2Ready / 调试用。
// 列表为空 ⇔ EnsureV2Ready 是 no-op。
func (s *v2State) missing() []string {
	var out []string

	hasUSDCE := s.usdcBal != nil && s.usdcBal.Sign() > 0
	if hasUSDCE {
		out = append(out, fmt.Sprintf("USDC.e=%s 待 wrap 成 pUSD", s.usdcBal.String()))
		// 只有需要 wrap 时才关心 Onramp allowance；够 wrap 就行，不必到 MAX
		if s.onrampAllow == nil || s.onrampAllow.Cmp(s.usdcBal) < 0 {
			out = append(out, "USDC.e→CollateralOnramp approve 不足 (将一并补)")
		}
	}
	for _, sp := range pusdSpendersV2() {
		if v := s.pusdAllow[sp]; v == nil || v.Sign() == 0 {
			out = append(out, fmt.Sprintf("pUSD→%s approve 缺", sp.Hex()))
		}
	}
	for _, op := range ctfOperatorsV2() {
		if !s.ctfAppr[op] {
			out = append(out, fmt.Sprintf("CTF.setApprovalForAll(%s) 缺", op.Hex()))
		}
	}
	return out
}

// buildEnsureBatch 根据 state 生成需要发的 proxy tx 列表（缺啥补啥，按依赖顺序）。
// 顺序：先 USDC.e→Onramp approve（如缺），再 wrap，再 pUSD approves，再 CTF approvals。
func (c *GaslessClient) buildEnsureBatch(safe common.Address, state *v2State) ([]map[string]any, error) {
	usdc := common.HexToAddress(internal.PolygonCollateral)
	pusd := common.HexToAddress(internal.PolygonPUSD)
	ctf := common.HexToAddress(internal.PolygonConditionalTokens)
	onramp := common.HexToAddress(internal.PolygonCollateralOnramp)

	var txns []map[string]any

	// 1) USDC.e → wrap → pUSD（仅在 Safe 上有 USDC.e 时执行）
	if state.usdcBal != nil && state.usdcBal.Sign() > 0 {
		// 1a) Onramp allowance 不够本次 wrap 量就**精确 approve**（不给 MAX，防止长期暴露）。
		//     Onramp 只在用户从 USDC.e 进场时被调，频率低，多 1 笔 approve 的成本换取
		//     "wrap 完 allowance 自动归零"的紧 scope，划算。
		if state.onrampAllow == nil || state.onrampAllow.Cmp(state.usdcBal) < 0 {
			data, err := erc20ABI.Pack("approve", onramp, state.usdcBal)
			if err != nil {
				return nil, fmt.Errorf("pack approve USDC.e→onramp: %w", err)
			}
			txns = append(txns, callTxn(usdc, data))
		}
		// 1b) wrap 全部 USDC.e 余额到 Safe（to=safe，1:1 铸 pUSD）
		wrapData, err := collateralOnrampABI.Pack("wrap", usdc, safe, state.usdcBal)
		if err != nil {
			return nil, fmt.Errorf("pack wrap: %w", err)
		}
		txns = append(txns, callTxn(onramp, wrapData))
	}

	// 2) pUSD allowances 缺啥补啥
	for _, sp := range pusdSpendersV2() {
		if v := state.pusdAllow[sp]; v == nil || v.Sign() == 0 {
			data, err := packApproveMax(sp)
			if err != nil {
				return nil, fmt.Errorf("pack approve pUSD→%s: %w", sp.Hex(), err)
			}
			txns = append(txns, callTxn(pusd, data))
		}
	}

	// 3) CTF.setApprovalForAll 缺哪个补哪个
	for _, op := range ctfOperatorsV2() {
		if !state.ctfAppr[op] {
			data, err := packSetApprovalForAll(op)
			if err != nil {
				return nil, fmt.Errorf("pack setApprovalForAll %s: %w", op.Hex(), err)
			}
			txns = append(txns, callTxn(ctf, data))
		}
	}

	return txns, nil
}
