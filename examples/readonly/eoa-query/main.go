// eoa_query：EOA 钱包全景：链上余额 + CLOB 活跃挂单 + data-api 持仓。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/clob"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const erc20Bal = `[{"name":"balanceOf","type":"function","stateMutability":"view","inputs":[{"name":"a","type":"address"}],"outputs":[{"type":"uint256"}]}]`

type position struct {
	Asset       string  `json:"asset"`
	ConditionID string  `json:"conditionId"`
	Outcome     string  `json:"outcome"`
	Size        float64 `json:"size"`
	AvgPrice    float64 `json:"avgPrice"`
	CurPrice    float64 `json:"curPrice"`
	CashPnl     float64 `json:"cashPnl"`
	Title       string  `json:"title"`
}

func main() {
	priv := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if priv == "" {
		log.Fatalf("缺少 poly_sec / POLY_PRIVATE_KEY")
	}

	w3, err := web3.NewClient(priv, types.EOASignatureType, types.Polygon)
	if err != nil {
		log.Fatalf("web3 client: %v", err)
	}
	defer w3.Close()
	eoa := common.HexToAddress(string(w3.GetBaseAddress()))
	fmt.Printf("EOA: %s\n\n", eoa.Hex())

	// === on-chain balances ===
	ctx := context.Background()
	rpc, err := ethclient.DialContext(ctx, "https://polygon-bor-rpc.publicnode.com")
	if err != nil {
		log.Fatalf("rpc dial: %v", err)
	}
	defer rpc.Close()
	a, _ := abi.JSON(strings.NewReader(erc20Bal))
	pol, _ := rpc.BalanceAt(ctx, eoa, nil)
	usdc := readBal(ctx, rpc, a, common.HexToAddress(internal.PolygonCollateral), eoa)
	pusd := readBal(ctx, rpc, a, common.HexToAddress(internal.PolygonPUSD), eoa)
	fmt.Println("=== On-chain Balances ===")
	fmt.Printf(" POL    : %.6f\n", weiToFloat(pol, 18))
	fmt.Printf(" USDC.e : %.6f\n", usdc)
	fmt.Printf(" pUSD   : %.6f\n\n", pusd)

	// === CLOB live orders ===
	client, err := clob.NewClient(w3, clob.WithV2())
	if err != nil {
		log.Fatalf("clob: %v", err)
	}
	orders, err := client.GetOrders(nil, nil, nil)
	if err != nil {
		fmt.Printf("⚠ GetOrders: %v\n\n", err)
	} else {
		fmt.Printf("=== Live Orders (%d) ===\n", len(orders))
		if len(orders) == 0 {
			fmt.Println(" (none)")
		}
		for _, o := range orders {
			fmt.Printf(" %-4s  %-3s  size=%-5g @ %-5g  filled=%-5g  market=%s…  token=%s…\n",
				o.Side, o.Outcome, float64(o.OriginalSize), float64(o.Price), float64(o.SizeMatched),
				shortHex(string(o.ConditionID)), shortToken(o.TokenID))
		}
		fmt.Println()
	}

	// === data-api positions ===
	fmt.Println("=== Positions (data-api, EOA) ===")
	pos, err := fetchPositions(eoa.Hex())
	if err != nil {
		fmt.Printf(" ⚠ %v\n", err)
	} else if len(pos) == 0 {
		fmt.Println(" (none)")
	} else {
		for _, p := range pos {
			fmt.Printf(" %-4s  size=%-10.4f avg=%-7.4f cur=%-7.4f pnl=%+8.4f  %s\n",
				p.Outcome, p.Size, p.AvgPrice, p.CurPrice, p.CashPnl, truncStr(p.Title, 60))
		}
	}
}

func readBal(ctx context.Context, rpc *ethclient.Client, a abi.ABI, token, owner common.Address) float64 {
	d, _ := a.Pack("balanceOf", owner)
	out, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &token, Data: d}, nil)
	if err != nil {
		return -1
	}
	var v *big.Int
	a.UnpackIntoInterface(&v, "balanceOf", out)
	return weiToFloat(v, 6)
}

func weiToFloat(v *big.Int, dec int) float64 {
	if v == nil {
		return 0
	}
	f, _ := new(big.Float).Quo(
		new(big.Float).SetInt(v),
		new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(dec)), nil)),
	).Float64()
	return f
}

func fetchPositions(addr string) ([]position, error) {
	url := "https://data-api.polymarket.com/positions?user=" + addr + "&sizeThreshold=0.000001&limit=500"
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out []position
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode: %w body=%s", err, truncStr(string(body), 200))
	}
	return out, nil
}

func shortHex(h string) string {
	if len(h) <= 14 {
		return h
	}
	return h[:6] + "…" + h[len(h)-4:]
}
func shortToken(t string) string {
	if len(t) <= 10 {
		return t
	}
	return t[:6]
}
func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
