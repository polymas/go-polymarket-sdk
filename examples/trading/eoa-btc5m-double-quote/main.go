// eoa_btc5m_double_quote：EOA 钱包以 1¢ 在 BTC 5min/15min 市场两边各挂 5 share BUY。
//
// 必选 env：poly_sec（或 POLY_PRIVATE_KEY），且 EOA 已跑过 examples/onchain/v2-eoa-approve
// 可选 env：POLY_TARGET_SLUG（指定具体市场 slug，默认取最新 btc-updown-5m）
// 强制 EOA 模式：无视 POLY_SIGNATURE_TYPE。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/polymas/go-polymarket-sdk/clob"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

const (
	gammaURL = "https://gamma-api.polymarket.com/markets?closed=false&limit=50&order=startDate&ascending=false&active=true"
	price    = 0.01
	size     = 5.0
)

type gammaMarket struct {
	Slug         string `json:"slug"`
	Question     string `json:"question"`
	StartDate    string `json:"startDate"`
	EndDate      string `json:"endDate"`
	ConditionID  string `json:"conditionId"`
	ClobTokenIds string `json:"clobTokenIds"` // gamma 返回的是 JSON 字符串
	Outcomes     string `json:"outcomes"`
}

func main() {
	priv := firstEnv("poly_sec", "POLY_PRIVATE_KEY")
	if priv == "" {
		log.Fatalf("缺少 poly_sec / POLY_PRIVATE_KEY")
	}

	var (
		mkt      *gammaMarket
		ids      [2]string
		outcomes [2]string
		err      error
	)
	if slug := os.Getenv("POLY_TARGET_SLUG"); slug != "" {
		mkt, ids, outcomes, err = findBySlug(slug)
	} else {
		mkt, ids, outcomes, err = findLatestBTC5m()
	}
	if err != nil {
		log.Fatalf("找市场失败: %v", err)
	}
	fmt.Println("===========================================================")
	fmt.Println(" EOA double-quote on latest BTC 5m market")
	fmt.Printf(" slug      : %s\n", mkt.Slug)
	fmt.Printf(" question  : %s\n", mkt.Question)
	fmt.Printf(" cond_id   : %s\n", mkt.ConditionID)
	fmt.Printf(" start/end : %s → %s\n", mkt.StartDate, mkt.EndDate)
	fmt.Printf(" tokens    : [%s] %s\n             [%s] %s\n",
		outcomes[0], shortID(ids[0]), outcomes[1], shortID(ids[1]))
	fmt.Printf(" plan      : BUY %.2f share @ %.2f¢ on each side  (notional %.4f USDC total)\n",
		size, price*100, 2*price*size)
	fmt.Println("===========================================================")

	// 强制 EOA 签名（sigType=0），覆盖 .env 里的 POLY_SIGNATURE_TYPE
	w3, err := web3.NewClient(priv, types.EOASignatureType, types.Polygon)
	if err != nil {
		log.Fatalf("web3 client: %v", err)
	}
	defer w3.Close()
	fmt.Printf("EOA: %s\n", w3.GetBaseAddress())

	client, err := clob.NewClient(w3, clob.WithV2())
	if err != nil {
		log.Fatalf("clob V2 client: %v", err)
	}

	t, err := client.GetTime()
	if err != nil {
		log.Fatalf("GetTime: %v", err)
	}
	fmt.Printf("server time: %s (skew %v)\n", t.UTC().Format(time.RFC3339), time.Since(t).Truncate(time.Second))

	// 强制刷新 CLOB 索引器：让服务端按链上最新状态重读 EOA 余额/授权。
	fmt.Println("refreshing CLOB balance/allowance index ...")
	if ba, err := client.UpdateBalanceAllowance(0); err != nil {
		fmt.Printf("  ⚠ UpdateBalanceAllowance: %v\n", err)
	} else if ba != nil {
		fmt.Printf("  collateral balance=%s allowance=%s\n", ba.Balance, ba.Allowance)
	}
	fmt.Println()

	for i, tokenID := range ids {
		side := outcomes[i]
		fmt.Printf("[%d/2] BUY %s (%s) @ 0.01 size 5 ...\n", i+1, side, shortID(tokenID))
		resp, err := client.PostOrder(types.OrderArgs{
			TokenID: tokenID,
			Price:   price,
			Size:    size,
			Side:    types.OrderSideBUY,
		}, types.OrderTypeGTC)
		if err != nil {
			fmt.Printf("    ❌ PostOrder err: %v\n\n", err)
			continue
		}
		if resp == nil {
			fmt.Printf("    ❌ nil response\n\n")
			continue
		}
		if resp.ErrorMsg != "" {
			fmt.Printf("    ❌ rejected: status=%s err=%s orderID=%s\n\n",
				resp.Status, resp.ErrorMsg, resp.OrderID)
			continue
		}
		fmt.Printf("    ✅ orderID=%s status=%s\n\n", resp.OrderID, resp.Status)
	}

	fmt.Println("done. 用以下命令查看活跃挂单：")
	fmt.Println("  POLY_SIGNATURE_TYPE=0 go run ./examples/trading/v2-cancel-one  # 列 + 撤")
}

func findBySlug(slug string) (*gammaMarket, [2]string, [2]string, error) {
	url := "https://gamma-api.polymarket.com/markets?slug=" + slug
	resp, err := http.Get(url)
	if err != nil {
		return nil, [2]string{}, [2]string{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, [2]string{}, [2]string{}, err
	}
	var arr []gammaMarket
	if err := json.Unmarshal(body, &arr); err != nil {
		return nil, [2]string{}, [2]string{}, fmt.Errorf("decode gamma: %w (body=%s)", err, truncBody(body))
	}
	if len(arr) == 0 {
		return nil, [2]string{}, [2]string{}, fmt.Errorf("slug %s 没找到", slug)
	}
	m := arr[0]
	var ids []string
	if err := json.Unmarshal([]byte(m.ClobTokenIds), &ids); err != nil {
		return nil, [2]string{}, [2]string{}, fmt.Errorf("parse clobTokenIds: %w", err)
	}
	if len(ids) != 2 {
		return nil, [2]string{}, [2]string{}, fmt.Errorf("expected 2 tokens, got %d", len(ids))
	}
	var outs []string
	if err := json.Unmarshal([]byte(m.Outcomes), &outs); err != nil || len(outs) != 2 {
		outs = []string{"YES", "NO"}
	}
	return &m, [2]string{ids[0], ids[1]}, [2]string{outs[0], outs[1]}, nil
}

func findLatestBTC5m() (*gammaMarket, [2]string, [2]string, error) {
	resp, err := http.Get(gammaURL)
	if err != nil {
		return nil, [2]string{}, [2]string{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, [2]string{}, [2]string{}, err
	}
	var arr []gammaMarket
	if err := json.Unmarshal(body, &arr); err != nil {
		return nil, [2]string{}, [2]string{}, fmt.Errorf("decode gamma: %w (body=%s)", err, truncBody(body))
	}
	for _, m := range arr {
		if !strings.Contains(strings.ToLower(m.Slug), "btc-updown-5m") {
			continue
		}
		var ids []string
		if err := json.Unmarshal([]byte(m.ClobTokenIds), &ids); err != nil {
			return nil, [2]string{}, [2]string{}, fmt.Errorf("parse clobTokenIds: %w", err)
		}
		if len(ids) != 2 {
			return nil, [2]string{}, [2]string{}, fmt.Errorf("expected 2 tokens, got %d", len(ids))
		}
		var outs []string
		if err := json.Unmarshal([]byte(m.Outcomes), &outs); err != nil || len(outs) != 2 {
			outs = []string{"YES", "NO"}
		}
		return &m, [2]string{ids[0], ids[1]}, [2]string{outs[0], outs[1]}, nil
	}
	return nil, [2]string{}, [2]string{}, fmt.Errorf("最近 20 条市场里没有 btc-updown-5m")
}

func shortID(id string) string {
	if len(id) <= 14 {
		return id
	}
	return id[:6] + "…" + id[len(id)-4:]
}

func truncBody(b []byte) string {
	if len(b) > 200 {
		return string(b[:200]) + "..."
	}
	return string(b)
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
