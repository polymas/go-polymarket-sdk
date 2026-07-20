# V2 业务层接入指南

Polymarket V2（2026-04-28 切换）下，collateral 从 USDC.e 改为 pUSD，订单/赎回路径都经 `CtfCollateralAdapter`。本文档面向**业务层接入方**，描述 SDK 提供的"自检 + 自动修复"接口，业务层只需几行就能完成 session bootstrap。

---

## 总览

```
                业务层 App 入口
                      │
                      ▼
    ┌─────────────────────────────┐
    │ Stage 1: 资金 + 授权自检   │
    │  gasless.IsV2Ready()        │   只读 ~300ms
    └────────┬────────────────────┘
             │ ready=false
             ▼
    ┌─────────────────────────────┐
    │  gasless.EnsureV2Ready()    │   缺啥补啥，1 笔 gasless batch
    └────────┬────────────────────┘
             │
             ▼
    ┌─────────────────────────────┐
    │ Stage 2: auto-claim 偏好    │
    │  IsAutoClaimEnabled() →     │
    │  Enable / Disable AutoClaim │
    └────────┬────────────────────┘
             │
             ▼
    ┌─────────────────────────────┐
    │ Stage 3: 业务自检（余额/    │
    │   allowance 缓存 / 时钟）   │
    └────────┬────────────────────┘
             │
             ▼
    ✓ 进入下单 / 持仓 / 赎回
```

---

## Stage 1：资金 + 授权自检

**目标**：确保 Safe 上没有未 wrap 的 USDC.e，且 V2 trading 全套 approve 到位。

### `gasless.IsV2Ready() (ready bool, missing []string, err error)`

只读检查 9 项 V2 trading 前置条件（11 笔 eth_call 并发，~300ms）：
- USDC.e 余额（>0 视为待 wrap）
- USDC.e → CollateralOnramp allowance
- pUSD → 4 个 spender allowance（V2 Exchange / V2 NegRisk Exchange / CtfCollateralAdapter / NegRiskCtfCollateralAdapter）；旧 NegRiskAdapter 仅供历史只读排查
- CTF.isApprovedForAll → 4 个 operator（V2 Exchange / V2 NegRisk Exchange / 两个 adapter）

返回 `ready=true` 时 `EnsureV2Ready()` 是 no-op；`false` 时 `missing[]` 列出待修复项，可直接展示给用户。

### `gasless.EnsureV2Ready() (*types.TransactionReceipt, error)`

幂等"缺啥补啥"：
1. Safe 有 USDC.e → 精确量 approve `CollateralOnramp` + wrap → pUSD（无 USDC.e 跳过）
2. pUSD 4 个有效 V2 spender allowance 缺哪补哪 (`approve(MAX)`)
3. CTF 4 个 operator approval 缺哪补哪 (`setApprovalForAll(true)`)

所有需要的 op 打包成**一笔 gasless batch**：
- 全部已就位 → 返回 `(nil, nil)`，不发 tx
- 否则 → 发一笔 batch，返回 `receipt`

### 业务层调用模板

```go
ready, missing, err := gasless.IsV2Ready()
if err != nil {
    return fmt.Errorf("self-check: %w", err)
}
if !ready {
    log.Printf("待修复：%v", missing)
    receipt, err := gasless.EnsureV2Ready()
    if err != nil {
        return fmt.Errorf("ensure v2: %w", err)
    }
    if receipt != nil {
        log.Printf("已修复 tx=%s block=%d", receipt.TxHash, receipt.BlockNumber)
    }
}
```

**调用频率建议**：每次 session 入口跑一次。`IsV2Ready` 是只读 RPC，开销低。

---

## Stage 2：auto-claim 偏好对齐

**目标**：把链上 operator approval 状态对齐到业务/用户的偏好。开启后 Polymarket 后端在市场结算后自动 redeem 用户赢家仓位。

### `gasless.IsAutoClaimEnabled() (bool, error)`

只读查询当前 Safe 是否对 Polymarket 自动 redeem 服务（`0x05Cd9922...`）授予 CTF operator 权限。

### `gasless.EnableAutoClaim() / DisableAutoClaim() (*types.TransactionReceipt, error)`

幂等开关。已就位则直接返回 `(nil, nil)` 不发 tx。底层是 `CTF.setApprovalForAll(claimer, bool)`。

### `gasless.SetAutoClaim(enabled bool) (*types.TransactionReceipt, error)`

非幂等版，总会发 tx。一般业务层用幂等版即可。

### 业务层调用模板

```go
userWantsAutoClaim := userSettings.AutoClaim  // 业务自己的开关
if userWantsAutoClaim {
    if _, err := gasless.EnableAutoClaim(); err != nil {  // 已开则 no-op
        return err
    }
} else {
    if _, err := gasless.DisableAutoClaim(); err != nil { // 已关则 no-op
        return err
    }
}
```

**注意**：当前 `0x05Cd9922...` 是 V1 时期的 claimer，bytecode 验证只支持 USDC.e 输出。V2 切换后 Polymarket 可能部署新的 V2 claimer，地址常量更新会通过 SDK 升级释放，业务层代码不变。

---

## Stage 3：业务自身检查（SDK 不管）

每次下单前业务层应该做：

| 检查项 | API | 失败处理 |
|---|---|---|
| **pUSD 余额** ≥ 本次 notional | `gasless.GetPUSDBalance()` ✦ | UI 提示充值 / 减少下单量 |
| 服务端 allowance 缓存新鲜 | `clob.UpdateBalanceAllowance(0)` | 刚 EnsureV2Ready 后建议刷一次 |
| 目标市场未关闭 | Gamma `GET /markets?slug=...` 看 `acceptingOrders` | 跳过、换市场或等下个窗口 |
| 时钟偏差 < 5 min | `clob.GetTime()` 比较本地 | 大于阈值签名一定被拒，提醒 NTP |

### `gasless.GetPUSDBalance() (float64, error)` ✦ 新增

直接读链上 Safe 的 pUSD 余额（人类单位，1.0 = 1 pUSD）。**单笔 eth_call，不发 tx**。

```go
bal, err := gasless.GetPUSDBalance()
if err != nil { return err }
if bal < tradeNotional {
    return fmt.Errorf("余额不足，有 %.2f pUSD，需要 %.2f", bal, tradeNotional)
}
```

为什么不直接用 `clob.GetBalanceAllowance()` 服务端版？后者带缓存，刚 wrap 完可能还显示旧值；`GetPUSDBalance` 走链上，永远准确。

---

## Stage 4：交易后

| 场景 | auto-claim 开 | auto-claim 关 |
|---|---|---|
| 市场结算 | Polymarket 后端自动 redeem | 业务层调 `gasless.RedeemPositions(...)` |
| 用户主动平仓（不等结算） | 业务层调 `gasless.MergePositions(...)` | 同左 |

---

## 完整业务层模板

```go
package main

import (
    "fmt"
    "log"

    "github.com/polymas/go-polymarket-sdk/types"
    "github.com/polymas/go-polymarket-sdk/web3"
)

// bootstrapV2Session 业务层 session 进入时调一次。返回 nil 即代表已就绪。
func bootstrapV2Session(g *web3.GaslessClient, autoClaimPreferred bool) error {
    // Stage 1: 资金 + 授权
    ready, missing, err := g.IsV2Ready()
    if err != nil {
        return fmt.Errorf("self-check: %w", err)
    }
    if !ready {
        log.Printf("修复中：%v", missing)
        if _, err := g.EnsureV2Ready(); err != nil {
            return fmt.Errorf("ensure: %w", err)
        }
    }

    // Stage 2: auto-claim 偏好
    if autoClaimPreferred {
        if _, err := g.EnableAutoClaim(); err != nil {
            return fmt.Errorf("enable auto-claim: %w", err)
        }
    } else {
        if _, err := g.DisableAutoClaim(); err != nil {
            return fmt.Errorf("disable auto-claim: %w", err)
        }
    }
    return nil
}

// preflightTrade 每次下单前的业务自检。
func preflightTrade(g *web3.GaslessClient, notional float64) error {
    bal, err := g.GetPUSDBalance()
    if err != nil {
        return fmt.Errorf("read pUSD balance: %w", err)
    }
    if bal < notional {
        return fmt.Errorf("余额不足：有 %.6f pUSD，需要 %.6f", bal, notional)
    }
    return nil
}

func main() {
    privateKey := /* env var */ ""
    sigType := types.SafeSignatureType
    creds := /* POLY_BUILDER_* */ &types.ApiCreds{}

    gasless, err := web3.NewGaslessClient(privateKey, sigType, types.Polygon, creds)
    if err != nil {
        log.Fatal(err)
    }

    if err := bootstrapV2Session(gasless, true); err != nil {
        log.Fatal(err)
    }

    if err := preflightTrade(gasless, 5.0); err != nil {
        log.Fatal(err)
    }
    // ... 走 clob.PostOrder / gasless.SplitPosition 等
}
```

---

## 命令行 example 验证流程

仓库 `examples/` 下的小工具可手动核对每一步：

| 用途 | 命令 |
|---|---|
| 看当前所有 V2 状态 | `go run ./examples/v2_allowance_check` |
| 自检（不发 tx） | `go run ./examples/v2_ensure status` |
| 缺啥补啥 | `go run ./examples/v2_ensure ensure` |
| auto-claim 状态 | `go run ./examples/v2_auto_claim status` |
| auto-claim 开关 | `go run ./examples/v2_auto_claim enable\|disable` |
| pUSD 余额查询 | （直接调 `gasless.GetPUSDBalance()`，无独立 example） |
| split / merge / redeem | `go run ./examples/v2_split_merge_redeem <op> <conditionId> <args>` |

---

## 已确认的安全约束

- **所有 approve 都是 V2 trading 协议必需的**，无冗余 spender
- **USDC.e → Onramp** 用**精确量**（不是 MAX），wrap 完 allowance 自动归零
- **pUSD 5 个 spender + CTF 4 个 operator** 用 MAX，标准模式
- 所有 spender / operator 地址都在 `internal/contracts.go`，常量级，不接受外部输入
- gasless tx 走 Polymarket Relayer，HMAC 鉴权，Safe nonce 防重放

---

## 调用频率与开销

| 操作 | 链上写 | 链上读 | 服务端 | 频率建议 |
|---|---|---|---|---|
| `IsV2Ready` | 0 | 11 | 0 | 每次 session 进入 |
| `EnsureV2Ready` | ≤1 笔 batch | 11 | 0 | 仅 `IsV2Ready=false` 时 |
| `IsAutoClaimEnabled` | 0 | 1 | 0 | session 进入 / UI 状态显示 |
| `Enable/DisableAutoClaim` | ≤1 笔 | 1 | 0 | 用户改偏好时 |
| `GetPUSDBalance` | 0 | 1 | 0 | 每次下单前 |
