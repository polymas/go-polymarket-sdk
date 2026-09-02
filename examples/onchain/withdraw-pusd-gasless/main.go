// withdraw_pusd_gasless：用任意签名类型的 gasless relayer 路径提 pUSD。
// 不查 EOA POL（gasless 不需要），直接调 SDK 现成的 TransferERC20。
// 失败时 dump RelayHub 上链事件帮助定位。
package main

import (
	"bytes"
	"encoding/json"
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
	"github.com/polymas/go-polymarket-sdk/web3/relayer"
)

func main() {
	pk := os.Getenv("POLY_PRIVATE_KEY")
	to := os.Getenv("WITHDRAW_TO")
	if pk == "" || !common.IsHexAddress(strings.TrimSpace(to)) {
		log.Fatalf("缺 POLY_PRIVATE_KEY / WITHDRAW_TO")
	}
	sigType := types.SignatureType(getInt("POLY_SIGNATURE_TYPE", 1))
	amount := getFloat("WITHDRAW_AMOUNT_PUSD", 0)
	if amount <= 0 {
		log.Fatalf("需要 WITHDRAW_AMOUNT_PUSD>0")
	}
	useBridge := os.Getenv("USE_BRIDGE") == "1"

	creds := &types.ApiCreds{
		Key:        os.Getenv("POLY_API_KEY"),
		Secret:     os.Getenv("POLY_API_SECRET"),
		Passphrase: os.Getenv("POLY_API_PASSPHRASE"),
	}
	if creds.Key == "" || creds.Secret == "" || creds.Passphrase == "" {
		log.Fatalf("需要 POLY_API_KEY / SECRET / PASSPHRASE")
	}

	c, err := relayer.NewGaslessClient(pk, sigType, types.Polygon, creds)
	if err != nil {
		log.Fatalf("NewGaslessClient: %v", err)
	}
	defer c.Close()

	eoa := c.GetBaseAddress()
	proxyHex, _ := c.GetPolyProxyAddress()
	proxy := common.HexToAddress(string(proxyHex))
	recipient := common.HexToAddress(to)
	pUSD := common.HexToAddress(internal.PolygonPUSD)

	bal, _ := c.GetERC20BalanceOf(pUSD, proxy)
	fmt.Printf("EOA=%s\nProxy(type=%d)=%s\npUSD bal=%g\n",
		eoa, sigType, proxy.Hex(), float64(bal.Int64())/1e6)

	amountInt := new(big.Int).SetInt64(int64(amount * 1e6))
	if amountInt.Cmp(bal) > 0 {
		log.Fatalf("余额不够: %g < %g", float64(bal.Int64())/1e6, amount)
	}

	dest := recipient
	if useBridge {
		fmt.Println("\n→ 调 bridge.polymarket.com/withdraw 拿 deposit 地址 ...")
		builderCode := ""
		if len(creds.Passphrase) == 64 {
			builderCode = "0x" + creds.Passphrase
		}
		depAddrs, raw, err := callBridgeWithdraw(proxy.Hex(), "137", pUSD.Hex(), recipient.Hex(), builderCode)
		if err != nil {
			log.Fatalf("bridge: %v\n%s", err, raw)
		}
		dhex := depAddrs["evm"]
		if !common.IsHexAddress(dhex) {
			log.Fatalf("bridge 没返回 evm 地址: %s", raw)
		}
		dest = common.HexToAddress(dhex)
		fmt.Printf("deposit 地址: %s（桥会异步转给 %s）\n", dest.Hex(), recipient.Hex())
	}

	fmt.Printf("\n→ gasless TransferERC20 pUSD %g → %s\n", amount, dest.Hex())
	r, err := c.TransferERC20(pUSD, dest, amountInt)
	if err != nil {
		log.Fatalf("❌ %v", err)
	}
	fmt.Printf("✅ tx=%s block=%d status=%d\n", r.TxHash, r.BlockNumber, r.Status)
	fmt.Printf("https://polygonscan.com/tx/%s\n", r.TxHash)

	if rb, err := c.GetERC20BalanceOf(pUSD, recipient); err == nil {
		fmt.Printf("recipient pUSD: %g\n", float64(rb.Int64())/1e6)
	}
	if pb, err := c.GetERC20BalanceOf(pUSD, proxy); err == nil {
		fmt.Printf("proxy 剩余: %g\n", float64(pb.Int64())/1e6)
	}
}

func callBridgeWithdraw(src, chain, token, recipient, builder string) (map[string]string, string, error) {
	buf, _ := json.Marshal(map[string]string{
		"address": src, "toChainId": chain, "toTokenAddress": token, "recipientAddr": recipient,
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
	json.Unmarshal(raw, &parsed)
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

func getInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
func getFloat(k string, def float64) float64 {
	if v := os.Getenv(k); v != "" {
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return def
}
