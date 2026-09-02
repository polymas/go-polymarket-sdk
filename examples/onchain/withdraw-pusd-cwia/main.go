// withdraw_pusd_cwia：按朋友 demo 的"EOA 签 batch → DepositWallet 自验签 → 自转账"
// 思路提现 pUSD（CWIA / DepositWallet 模式，POLY_SIGNATURE_TYPE=3）。
//
// 流程：
//  1. 推导 EOA 对应的 DepositWallet 地址（DepositWalletFactory.predictWalletAddress）
//  2. 链上探测：钱包是否部署；pUSD 余额够不够
//  3. POST bridge.polymarket.com/withdraw 拿 deposit 地址（X-Builder-Code = POLY_API_PASSPHRASE）
//  4. 构造单笔 CWIA Batch：calls = [{ to: pUSD, data: transfer(deposit, amount) }]
//     EOA 用 EIP-712 + Solady ERC-1271 嵌套签名
//  5. POST relayer-v2.polymarket.com/submit（type=WALLET / depositWalletParams 结构）
//     DepositWallet 本身验证 EOA 签名，自己执行 pUSD.transfer
//
// 必选 env：
//
//	POLY_PRIVATE_KEY                EOA 私钥（必须是 DepositWallet 的 signer/owner）
//	POLY_API_KEY/SECRET/PASSPHRASE  HMAC 三元组 + 桥的 builder code
//	WITHDRAW_TO                     最终收款地址
//
// 可选 env：
//
//	WITHDRAW_AMOUNT_PUSD            提现 pUSD 数量（人类单位）；未设 = 全余额
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	pk := firstEnv("POLY_PRIVATE_KEY", "poly_sec")
	if pk == "" {
		log.Fatalf("缺少 POLY_PRIVATE_KEY")
	}
	to := strings.TrimSpace(os.Getenv("WITHDRAW_TO"))
	if !common.IsHexAddress(to) {
		log.Fatalf("WITHDRAW_TO 不是合法地址: %q", to)
	}
	recipient := common.HexToAddress(to)

	creds := &types.ApiCreds{
		Key:        os.Getenv("POLY_API_KEY"),
		Secret:     os.Getenv("POLY_API_SECRET"),
		Passphrase: os.Getenv("POLY_API_PASSPHRASE"),
	}
	if creds.Key == "" || creds.Secret == "" || creds.Passphrase == "" {
		log.Fatalf("POLY_API_KEY / SECRET / PASSPHRASE 必须齐备")
	}

	// 强制走 CWIA 路径（朋友 demo 的 DepositWallet 模式）
	c, err := web3.NewGaslessClient(pk, types.CWIASignatureType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}
	defer c.Close()

	eoa := c.GetBaseAddress()
	walletHex, err := c.GetPolyProxyAddress()
	if err != nil {
		log.Fatalf("推导 DepositWallet 地址失败: %v", err)
	}
	wallet := common.HexToAddress(string(walletHex))
	pUSD := common.HexToAddress(internal.PolygonPUSD)

	fmt.Println("============================================================")
	fmt.Println(" Polymarket pUSD 提现（CWIA / DepositWallet 模式）")
	fmt.Println("============================================================")
	fmt.Printf(" EOA            : %s\n", eoa)
	fmt.Printf(" DepositWallet  : %s\n", wallet.Hex())
	fmt.Printf(" Recipient      : %s\n", recipient.Hex())
	fmt.Println("============================================================")

	// 链上探测：钱包是否已部署
	deployed, err := c.IsDepositWalletDeployed(context.Background(), wallet)
	if err != nil {
		log.Fatalf("探测 DepositWallet 部署状态失败: %v", err)
	}
	if !deployed {
		fmt.Println("⚠️  DepositWallet 还未部署到链上。")
		fmt.Println("    先跑 GaslessClient.DeployDepositWallet()（type=WALLET-CREATE）部署。")
		if os.Getenv("AUTO_DEPLOY") == "1" {
			fmt.Println("\n→ AUTO_DEPLOY=1，自动调 DeployDepositWallet ...")
			r, err := c.DeployDepositWallet(false)
			if err != nil {
				log.Fatalf("DeployDepositWallet: %v", err)
			}
			fmt.Printf("✅ wallet 部署完成 tx=%s block=%d\n", r.TxHash, r.BlockNumber)
		} else {
			log.Fatalf("加 AUTO_DEPLOY=1 自动部署，或先手动部署再提现")
		}
	}

	// pUSD 余额
	balUnits, err := c.GetERC20BalanceOf(pUSD, wallet)
	if err != nil {
		log.Fatalf("查 DepositWallet pUSD 余额失败: %v", err)
	}
	balHuman := bfToFloat(new(big.Float).Quo(new(big.Float).SetInt(balUnits), big.NewFloat(1e6)))
	fmt.Printf("DepositWallet pUSD 余额: %.6f\n", balHuman)
	if balUnits.Sign() == 0 {
		log.Fatalf("DepositWallet 上没有 pUSD。\n" +
			"  说明：你的资金可能在历史 PolyProxy (sig_type=1) 钱包里。\n" +
			"  CWIA 模式（朋友 demo 的思路）只能提走 DepositWallet 上的 pUSD。")
	}

	// 提现金额
	amountInt := new(big.Int).Set(balUnits)
	if v := os.Getenv("WITHDRAW_AMOUNT_PUSD"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			log.Fatalf("WITHDRAW_AMOUNT_PUSD 解析失败: %v", err)
		}
		bf := new(big.Float).Mul(big.NewFloat(f), big.NewFloat(1e6))
		amountInt, _ = bf.Int(nil)
	}
	if amountInt.Sign() <= 0 || amountInt.Cmp(balUnits) > 0 {
		log.Fatalf("提现金额非法或超过余额：%s > %s", amountInt.String(), balUnits.String())
	}

	// 桥拿 deposit 地址
	builderCode := os.Getenv("POLY_BUILDER_CODE")
	if builderCode == "" && len(creds.Passphrase) == 64 {
		builderCode = "0x" + creds.Passphrase
	}
	depAddrs, raw, err := callBridgeWithdraw(wallet.Hex(), "137", pUSD.Hex(), recipient.Hex(), builderCode)
	if err != nil {
		log.Fatalf("bridge /withdraw 失败：%v\n  body: %s", err, raw)
	}
	depHex, ok := depAddrs["evm"]
	if !ok || !common.IsHexAddress(depHex) {
		log.Fatalf("bridge 未返回 evm deposit 地址；raw=%s", raw)
	}
	depositAddr := common.HexToAddress(depHex)
	fmt.Printf("Bridge deposit 地址: %s\n", depositAddr.Hex())

	// 走 SDK 的 CWIA gasless 路径：底层会
	//   1) GetDepositWalletNonce 链上读 nonce
	//   2) BuildAndSignCWIABatch EOA 用 EIP-712 + Solady ERC-1271 签 batch
	//   3) POST /submit type=WALLET → relayer 上链 factory.proxy([batch],[sig])
	//   4) DepositWallet 验证签名后自执行 pUSD.transfer(deposit, amount)
	fmt.Printf("\n→ CWIA gasless: pUSD.transfer(%s, %.6f) ...\n", depositAddr.Hex(), bfToFloat(new(big.Float).Quo(new(big.Float).SetInt(amountInt), big.NewFloat(1e6))))
	r, err := c.TransferERC20(pUSD, depositAddr, amountInt)
	if err != nil {
		if errors.Is(err, web3.ErrDepositWalletNotDeployed) {
			log.Fatalf("DepositWallet 不存在（再次确认 sig_type 是否正确）")
		}
		log.Fatalf("❌ CWIA 提现失败: %v", err)
	}
	fmt.Printf("✅ on-chain: tx=%s block=%d status=%d\n", r.TxHash, r.BlockNumber, r.Status)
	fmt.Printf("https://polygonscan.com/tx/%s\n", r.TxHash)

	// 桥后端会异步从 deposit 地址转给 recipient，几十秒到几分钟
	if dBal, err := c.GetERC20BalanceOf(pUSD, depositAddr); err == nil {
		fmt.Printf("deposit 地址当前 pUSD: %g（桥会异步搬走）\n", float64(dBal.Int64())/1e6)
	}
	if wBal, err := c.GetERC20BalanceOf(pUSD, wallet); err == nil {
		fmt.Printf("DepositWallet 剩余 pUSD: %g\n", float64(wBal.Int64())/1e6)
	}
	fmt.Println("\n桥会在几分钟内把资金按 recipientAddr 转给最终地址。可用 bridge.polymarket.com/transaction-status?... 查进度。")
}

func callBridgeWithdraw(src, chain, token, recipient, builder string) (map[string]string, string, error) {
	buf, _ := json.Marshal(map[string]string{
		"address":        src,
		"toChainId":      chain,
		"toTokenAddress": token,
		"recipientAddr":  recipient,
	})
	req, _ := http.NewRequest("POST", "https://bridge.polymarket.com/withdraw", bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if builder != "" {
		req.Header.Set("X-Builder-Code", builder)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, string(raw), fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, string(raw), err
	}
	out := map[string]string{}
	if addr, ok := parsed["address"].(map[string]any); ok {
		for k, v := range addr {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out, string(raw), nil
}

func bfToFloat(b *big.Float) float64 { v, _ := b.Float64(); return v }
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
