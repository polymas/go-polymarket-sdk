// withdraw_pusd_safe：Safe 钱包通过 SDK gasless relayer 提 pUSD 测试。
// Safe 的 relayer 走 execTransaction（owner EIP-712 签名 → relayer 代提交），
// 不经过 PolyProxyFactory.acceptRelayedCall 那条白名单链路。
package main

import (
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3/relayer"
)

func main() {
	pk := os.Getenv("POLY_PRIVATE_KEY")
	to := os.Getenv("WITHDRAW_TO")
	if pk == "" || !common.IsHexAddress(strings.TrimSpace(to)) {
		log.Fatalf("需要 POLY_PRIVATE_KEY + WITHDRAW_TO")
	}
	amountStr := os.Getenv("WITHDRAW_AMOUNT_PUSD")
	if amountStr == "" {
		amountStr = "5"
	}
	var amount float64
	fmt.Sscanf(amountStr, "%f", &amount)
	if amount <= 0 {
		log.Fatalf("invalid amount")
	}
	amountInt := new(big.Int).SetInt64(int64(amount * 1e6))

	creds := &types.ApiCreds{
		Key:        os.Getenv("POLY_API_KEY"),
		Secret:     os.Getenv("POLY_API_SECRET"),
		Passphrase: os.Getenv("POLY_API_PASSPHRASE"),
	}
	c, err := relayer.NewGaslessClient(pk, types.SafeSignatureType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}
	defer c.Close()

	eoa := c.GetBaseAddress()
	proxyStr, _ := c.GetPolyProxyAddress()
	proxy := common.HexToAddress(string(proxyStr))
	recipient := common.HexToAddress(to)
	pUSD := common.HexToAddress(internal.PolygonPUSD)

	bal, _ := c.GetERC20BalanceOf(pUSD, proxy)
	fmt.Printf("EOA=%s\nSafe=%s\npUSD bal=%s (=%g)\n",
		eoa, proxy.Hex(), bal.String(), float64(bal.Int64())/1e6)
	if bal.Cmp(amountInt) < 0 {
		log.Fatalf("Safe pUSD 不够")
	}

	fmt.Printf("\n→ Safe gasless pUSD.transfer(%s, %.6f) ...\n", recipient.Hex(), amount)
	r, err := c.TransferERC20(pUSD, recipient, amountInt)
	if err != nil {
		log.Fatalf("❌ %v", err)
	}
	fmt.Printf("tx=%s block=%d status=%d\n", r.TxHash, r.BlockNumber, r.Status)

	if rb, err := c.GetERC20BalanceOf(pUSD, recipient); err == nil {
		fmt.Printf("recipient pUSD: %g\n", float64(rb.Int64())/1e6)
	}
	if pb, err := c.GetERC20BalanceOf(pUSD, proxy); err == nil {
		fmt.Printf("Safe pUSD: %g\n", float64(pb.Int64())/1e6)
	}
}
