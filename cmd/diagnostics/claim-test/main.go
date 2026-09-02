// claim_test：诊断 + 真实数据测试 NegRisk / Regular 仓位 gasless redeem。
//
// 第一阶段（默认）：纯诊断，不发交易。打印：
//   - EOA 地址 + MATIC 余额
//   - 用 3 种签名类型分别派生 proxy 地址，每个地址链上是否部署 + 余额
//   - 通过 data-api 查到的 redeemable 仓位列表
//
// 第二阶段（带 EXEC=<conditionId>）：对指定 conditionId 跑一笔 redeem
//   - 用 SIGTYPE 环境变量选签名类型（1=Proxy, 2=Safe, 3=CWIA），默认 2
//   - 通过 SDK GaslessClient.RedeemPositions 走完整链路
//   - 打印请求体、响应、receipt
//
// 用法：
//
//	PK=0x...                                  go run ./cmd/diagnostics/claim-test   # 仅诊断
//	PK=0x... EXEC=0xabc... SIGTYPE=2          go run ./cmd/diagnostics/claim-test   # 跑一笔
//	RPC=https://polygon.drpc.org PK=... ...   go run ./cmd/diagnostics/claim-test   # 自定义 RPC
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3/relayer"
)

const dataAPI = "https://data-api.polymarket.com"

func main() {
	pk := strings.TrimSpace(os.Getenv("PK"))
	if pk == "" {
		log.Fatal("env PK is required (0x... EOA private key)")
	}
	priv, err := ethcrypto.HexToECDSA(strings.TrimPrefix(pk, "0x"))
	if err != nil {
		log.Fatalf("bad PK: %v", err)
	}
	eoa := ethcrypto.PubkeyToAddress(priv.PublicKey)
	rpcEnv := strings.TrimSpace(os.Getenv("RPC"))
	var rpcs []string
	if rpcEnv != "" {
		rpcs = strings.Split(rpcEnv, ",")
		for i := range rpcs {
			rpcs[i] = strings.TrimSpace(rpcs[i])
		}
	} else {
		// 默认走 QuickNode 公共节点，比 SDK 内置 onfinality 稳。
		rpcs = []string{"https://rpc-mainnet.matic.quiknode.pro"}
	}
	fmt.Println("================ Wallet Discovery ================")
	fmt.Printf("EOA: %s\n", eoa.Hex())
	if len(rpcs) > 0 {
		fmt.Printf("RPC: %s\n", strings.Join(rpcs, ", "))
	}

	// 三种 signature type，分别派生 proxy
	sigTypes := []struct {
		t    types.SignatureType
		name string
	}{
		{types.ProxySignatureType, "Proxy (1)"},
		{types.SafeSignatureType, "Safe (2)"},
		{types.CWIASignatureType, "CWIA (3)"},
	}

	var picked struct {
		client     *relayer.GaslessClient
		proxyAddr  string
		signatureT types.SignatureType
	}

	for _, st := range sigTypes {
		fmt.Printf("\n--- %s ---\n", st.name)
		c, err := relayer.NewGaslessClient(pk, st.t, types.Polygon, nil)
		if err != nil {
			fmt.Printf("  NewGaslessClient failed: %v\n", err)
			continue
		}
		a, e := c.GetPolyProxyAddress()
		if e != nil {
			fmt.Printf("  GetPolyProxyAddress: %v\n", e)
			continue
		}
		addr := string(a)
		fmt.Printf("  derived proxy: %s\n", addr)
		fmt.Printf("  data-api positions for this proxy:\n")
		printPositions(addr)

		// 选第一个能 list 出仓位的客户端作为后续 redeem 候选
		if picked.client == nil {
			picked.client = c
			picked.proxyAddr = addr
			picked.signatureT = st.t
		}
	}

	cond := strings.TrimSpace(os.Getenv("EXEC"))
	if cond == "" {
		fmt.Println("\n================ Diagnostic-only ================")
		fmt.Println("set EXEC=<conditionId> SIGTYPE=<1|2|3> to actually run a redeem.")
		return
	}

	// 第二阶段：跑一笔
	sigTypeStr := strings.TrimSpace(os.Getenv("SIGTYPE"))
	if sigTypeStr != "" {
		n, _ := strconv.Atoi(sigTypeStr)
		st := types.SignatureType(n)
		c, err := relayer.NewGaslessClient(pk, st, types.Polygon, nil, rpcs...)
		if err != nil {
			log.Fatalf("NewGaslessClient with SIGTYPE=%d: %v", n, err)
		}
		addr, err := c.GetPolyProxyAddress()
		if err != nil {
			log.Fatalf("GetPolyProxyAddress for SIGTYPE=%d: %v", n, err)
		}
		picked.client = c
		picked.signatureT = st
		picked.proxyAddr = string(addr)
	}
	if picked.client == nil {
		log.Fatal("no usable client (set SIGTYPE manually)")
	}

	// 查这个 conditionId 在持仓里的元数据
	fmt.Println("\n================ Execute Redeem ================")
	fmt.Printf("conditionId: %s\n", cond)
	fmt.Printf("sigType:     %d\n", picked.signatureT)
	fmt.Printf("proxy:       %s\n", picked.proxyAddr)

	pos := findPosition(picked.proxyAddr, cond)
	if pos == nil {
		log.Fatalf("conditionId %s 在 %s 的持仓里找不到", cond, picked.proxyAddr)
	}
	fmt.Printf("position:    outcome=%s idx=%d size=%f negRisk=%v redeemable=%v\n",
		pos.Outcome, pos.OutcomeIndex, pos.Size, pos.NegativeRisk, pos.Redeemable)
	if !pos.Redeemable {
		log.Fatalf("not redeemable yet")
	}

	info := relayer.RedeemPositionInfo{
		ConditionID:  types.Keccak256(cond),
		OutcomeIndex: pos.OutcomeIndex,
		Size:         pos.Size,
		NegRisk:      pos.NegativeRisk,
	}
	fmt.Printf("calling RedeemPositions(%+v)\n", info)

	start := time.Now()
	receipt, err := picked.client.RedeemPositions([]relayer.RedeemPositionInfo{info})
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("❌ redeem FAILED after %v: %v\n", elapsed, err)
		return
	}
	fmt.Printf("✅ redeem OK after %v\n", elapsed)
	fmt.Printf("   tx hash: %s\n", receipt.TxHash)
	fmt.Printf("   block:   %d\n", receipt.BlockNumber)
	fmt.Printf("   status:  %d\n", receipt.Status)
	fmt.Printf("   gasUsed: %d\n", receipt.GasUsed)
}

func printPositions(addr string) {
	posList, err := fetchPositions(addr)
	if err != nil {
		fmt.Printf("    fetch err: %v\n", err)
		return
	}
	if len(posList) == 0 {
		fmt.Printf("    (no positions)\n")
		return
	}
	redeemCount := 0
	for _, p := range posList {
		marker := " "
		if p.Redeemable {
			marker = "R"
			redeemCount++
		}
		nr := "  "
		if p.NegativeRisk {
			nr = "NR"
		}
		title := p.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Printf("    [%s] %s outcome=%-3s idx=%d size=%-12.4f cond=%s | %s\n",
			marker, nr, p.Outcome, p.OutcomeIndex, p.Size, string(p.ConditionID)[:10]+"...", title)
	}
	fmt.Printf("    total=%d, redeemable=%d\n", len(posList), redeemCount)
}

func fetchPositions(addr string) ([]types.Position, error) {
	url := fmt.Sprintf("%s/positions?user=%s&sizeThreshold=0.01&limit=200", dataAPI, addr)
	req, _ := http.NewRequest("GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var out []types.Position
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func findPosition(addr, cond string) *types.Position {
	posList, err := fetchPositions(addr)
	if err != nil {
		return nil
	}
	for i, p := range posList {
		if strings.EqualFold(string(p.ConditionID), cond) {
			return &posList[i]
		}
	}
	return nil
}
