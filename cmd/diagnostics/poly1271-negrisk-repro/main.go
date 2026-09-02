// Command poly1271_negrisk_repro 在链上复刻 POLY_1271（NegRisk 签名域错误）的
// 根因与修复，全程只读 eth_call，不 POST 订单、不需要 API key、零资金风险。
//
// 背景：type-3（CWIA）订单的 V2 签名是 solady ERC-7739 嵌套签名，内部绑定了
// 签名时用的 appDomain（=某个 Exchange 的 EIP-712 域）。服务端拿到订单后，按
// 市场真实的 NegRisk 状态选 Exchange 域算 digest_server，再调
// wallet.isValidSignature(digest_server, sig)。只有"签名时用的域 == 服务端
// 选的域"时钱包才返回 magic value 0x1626ba7e；否则返回非 magic / revert，
// 服务端即报 "invalid POLY_1271 signature: signature does not match order hash"。
//
// 旧 SDK 对 NegRisk 市场写死 negRisk=false（用 ExchangeV2 域签），于是 NegRisk
// 市场的 1271 钱包单子永久失败。本程序对一个真实 NegRisk 市场 token：
//
//	[改动前] 用 ExchangeV2     域签 → isValidSignature(digest_server) ≠ magic  ❌ 复现
//	[改动后] 用 NegRiskExchange 域签 → isValidSignature(digest_server) = magic  ✅ 修复
//	[对照]   非 NegRisk 市场 + ExchangeV2 域签 → = magic                       ✅ 普通市场本就正常
//
// 运行：
//
//	set -a; source .env; set +a      # .env 里 POLY_PRIVATE_KEY = type-3 钱包私钥
//	go run ./cmd/diagnostics/poly1271-negrisk-repro
//
// 可选环境变量覆盖默认 token：
//
//	NEGRISK_TOKEN_ID=...   一个 neg_risk=true 市场的 outcome tokenID
//	NORMAL_TOKEN_ID=...    一个 neg_risk=false 市场的 outcome tokenID
package main

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/polymas/go-polymarket-sdk/internal"
	http "github.com/polymas/go-polymarket-sdk/internal/transport"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// 两个 V2 Exchange 地址（来自 internal/contracts.go，这里直接引用常量）。
var (
	exchangeV2        = common.HexToAddress(internal.PolygonExchangeV2)        // 普通
	negRiskExchangeV2 = common.HexToAddress(internal.PolygonNegRiskExchangeV2) // NegRisk
)

// eip1271MagicValue 是 EIP-1271 校验通过的返回值（isValidSignature 的 selector）。
var eip1271MagicValue = common.FromHex("0x1626ba7e")

// 公共 Polygon RPC（很多公共节点禁 eth_call/限流，用这几个相对稳的）。
var rpcURLs = []string{
	"https://polygon-pokt.nodies.app",
	"https://1rpc.io/matic",
	"https://polygon.gateway.tenderly.co",
}

// 默认 token（可被环境变量覆盖）。若失效请从 gamma /markets 里另找：
//   - NegRisk: 找 negRisk=true 的市场任取一个 clobTokenId
//   - 普通:    找 negRisk=false 的市场任取一个 clobTokenId
const (
	defaultNegRiskTokenID = "" // 留空 → 强制要求用 NEGRISK_TOKEN_ID 传入
	defaultNormalTokenID  = "" // 留空 → 跳过对照组
)

func main() {
	pk := os.Getenv("POLY_PRIVATE_KEY")
	if pk == "" {
		fatal("缺少 POLY_PRIVATE_KEY，请先 `set -a; source .env; set +a`")
	}
	negRiskTokenID := envOr("NEGRISK_TOKEN_ID", defaultNegRiskTokenID)
	normalTokenID := envOr("NORMAL_TOKEN_ID", defaultNormalTokenID)
	if negRiskTokenID == "" {
		fatal("缺少 NegRisk tokenID：请设置 NEGRISK_TOKEN_ID 为一个 neg_risk=true 市场的 clobTokenId")
	}

	// 1) 用私钥建 type-3 web3 client，拿 deposit wallet 地址 maker。
	w3, err := web3.NewClient(pk, types.CWIASignatureType, types.Polygon, rpcURLs...)
	if err != nil {
		fatal("web3.NewClient: %v", err)
	}
	makerAddrStr, err := w3.GetPolyProxyAddress()
	if err != nil {
		fatal("GetPolyProxyAddress: %v", err)
	}
	maker := common.HexToAddress(string(makerAddrStr))
	signer := w3.GetSigner()

	// 自建一个 eth_call 用的 ethclient（独立于 web3 内部连接，逻辑更透明）。
	rpc := dialFirst(rpcURLs)
	if rpc == nil {
		fatal("无法连接任何 RPC 节点: %v", rpcURLs)
	}
	defer rpc.Close()

	// 2) 确认两个 token 的真实 NegRisk 状态（只读 GET /neg-risk，不需要 API key）。
	negRiskActual := mustNegRisk(negRiskTokenID)
	if !negRiskActual {
		fatal("NEGRISK_TOKEN_ID=%s 的 neg_risk 实际为 false，请换一个 neg_risk=true 的 token", negRiskTokenID)
	}

	fmt.Println("==================== POLY_1271 NegRisk 签名域复刻 ====================")
	fmt.Printf("NegRisk 市场 tokenID = %s  (GET /neg-risk => true)\n", negRiskTokenID)
	fmt.Printf("maker(deposit wallet) = %s\n", maker.Hex())
	fmt.Printf("ExchangeV2          = %s\n", exchangeV2.Hex())
	fmt.Printf("NegRiskExchangeV2   = %s\n", negRiskExchangeV2.Hex())
	fmt.Println("--------------------------------------------------------------------")

	// 固定 Salt / TimestampMS，保证可复现。
	fixedSalt := big.NewInt(1)
	fixedTimestampMS := int64(1_700_000_000_000)

	// 3) 构造 NegRisk 市场的订单数据（Maker=Signer=maker，CWIA）。
	negRiskData := &signing.V2OrderData{
		Maker:         maker.Hex(),
		Signer:        maker.Hex(),
		TokenID:       negRiskTokenID,
		MakerAmount:   "5000000", // 5 USDC（任意合法值，只影响 structHash，不影响结论）
		TakerAmount:   "10000000",
		Side:          signing.V2SideBUY,
		SignatureType: types.CWIASignatureType,
		TimestampMS:   fixedTimestampMS,
		Salt:          fixedSalt,
	}

	// 4) 服务端视角：NegRisk 市场 → digest_server 用 NegRiskExchangeV2 域。
	//    先用 fixed 行为签一次以拿到确定的 V2Order，再据此算 digest_server
	//    （digest 只依赖订单字段 + 域，不依赖签名，所以两种签法的 V2Order 一致）。
	signedFixed, err := signing.BuildSignedV2OrderWithSigner(signer, negRiskData, big.NewInt(137), negRiskExchangeV2)
	if err != nil {
		fatal("签名(fixed/NegRiskExchangeV2): %v", err)
	}
	digestServer, err := signing.V2OrderDigest(&signedFixed.V2Order, big.NewInt(137), negRiskExchangeV2)
	if err != nil {
		fatal("V2OrderDigest(server/NegRisk): %v", err)
	}

	// 改动前（buggy）：写死 negRisk=false → 用 ExchangeV2 域签。
	signedBuggy, err := signing.BuildSignedV2OrderWithSigner(signer, negRiskData, big.NewInt(137), exchangeV2)
	if err != nil {
		fatal("签名(buggy/ExchangeV2): %v", err)
	}

	// 5) 两种签名分别对同一个 digest_server 跑 isValidSignature。
	buggyOK, buggyDesc := isValidSignature(rpc, maker, digestServer, signedBuggy.Signature)
	fixedOK, fixedDesc := isValidSignature(rpc, maker, digestServer, signedFixed.Signature)

	fmt.Printf("[改动前 用 ExchangeV2 签]        isValidSignature => %s  %s\n",
		buggyDesc, verdict(!buggyOK, "✅ 复现 POLY_1271（域不匹配，非 magic）", "⚠️ 未复现（预期应失败）"))
	fmt.Printf("[改动后 用 NegRiskExchangeV2 签] isValidSignature => %s  %s\n",
		fixedDesc, verdict(fixedOK, "✅ 修复（域匹配，返回 magic）", "❌ 未修复（预期应返回 magic）"))

	// 6) 对照组（可选）：非 NegRisk 市场，digest_server 用 ExchangeV2，buggy 行为
	//    （ExchangeV2 签）应返回 magic —— 说明普通市场本就没事，只有 NegRisk 中招。
	if normalTokenID != "" {
		if mustNegRisk(normalTokenID) {
			fmt.Printf("[对照] NORMAL_TOKEN_ID=%s 实际是 NegRisk，跳过对照组\n", normalTokenID)
		} else {
			normalData := *negRiskData
			normalData.TokenID = normalTokenID
			signedNormal, err := signing.BuildSignedV2OrderWithSigner(signer, &normalData, big.NewInt(137), exchangeV2)
			if err != nil {
				fatal("签名(对照/ExchangeV2): %v", err)
			}
			digestNormalServer, err := signing.V2OrderDigest(&signedNormal.V2Order, big.NewInt(137), exchangeV2)
			if err != nil {
				fatal("V2OrderDigest(对照): %v", err)
			}
			okN, descN := isValidSignature(rpc, maker, digestNormalServer, signedNormal.Signature)
			fmt.Printf("[对照 非NegRisk市场 用 ExchangeV2 签] isValidSignature => %s  %s\n",
				descN, verdict(okN, "✅ 普通市场本就正常", "⚠️ 异常（预期应返回 magic）"))
		}
	} else {
		fmt.Println("[对照] 未设置 NORMAL_TOKEN_ID，跳过非 NegRisk 对照组")
	}

	// 7) 纯逻辑断言：重试判据 bug 与修复（不联网，印证服务端错误串走向）。
	fmt.Println("--------------------------------------------------------------------")
	const serverErr = "invalid POLY_1271 signature: signature does not match order hash"
	oldHit := contains(serverErr, "invalid signature")
	newHit := contains(serverErr, "POLY_1271") || contains(serverErr, "does not match order hash")
	fmt.Printf("重试判据: 旧(\"invalid signature\")=%v  新(POLY_1271/does not match)=%v\n", oldHit, newHit)
	if oldHit {
		fatal("前提失效：服务端串竟含连续子串 \"invalid signature\"")
	}
	if !newHit {
		fatal("修复失效：新判据未能命中服务端串")
	}
	fmt.Println("====================================================================")
}

// isValidSignature 调 maker 钱包的 EIP-1271 isValidSignature(bytes32,bytes)，
// 返回是否等于 magic value 以及人类可读描述。revert 视为非 magic（域不匹配
// 时 solady 会 revert 或返回非 magic）。
func isValidSignature(rpc *ethclient.Client, wallet common.Address, hash common.Hash, sig []byte) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	data := buildIsValidSig(hash, sig)
	out, err := rpc.CallContract(ctx, ethereum.CallMsg{To: &wallet, Data: data}, nil)
	if err != nil {
		return false, fmt.Sprintf("revert/error(%v)", err)
	}
	if len(out) >= 4 && string(out[:4]) == string(eip1271MagicValue) {
		return true, "0x1626ba7e"
	}
	if len(out) == 0 {
		return false, "0x(empty)"
	}
	return false, "0x" + common.Bytes2Hex(out[:min(4, len(out))])
}

// buildIsValidSig ABI 编码 isValidSignature(bytes32 hash, bytes signature)。
func buildIsValidSig(hash common.Hash, sig []byte) []byte {
	out := common.FromHex("0x1626ba7e")
	out = append(out, hash.Bytes()...)
	off := make([]byte, 32)
	off[31] = 0x40 // bytes 参数 offset = 0x40（紧跟 hash 之后）
	out = append(out, off...)
	l := make([]byte, 32)
	big.NewInt(int64(len(sig))).FillBytes(l)
	out = append(out, l...)
	out = append(out, sig...)
	if pad := (32 - len(sig)%32) % 32; pad > 0 {
		out = append(out, make([]byte, pad)...)
	}
	return out
}

// mustNegRisk 查询单个 token 的 NegRisk 状态（GET /neg-risk，无需鉴权）。
func mustNegRisk(tokenID string) bool {
	resp, err := http.Get[struct {
		NegRisk bool `json:"neg_risk"`
	}](internal.ClobAPIDomain, internal.GetNegRisk, map[string]string{"token_id": tokenID})
	if err != nil {
		fatal("GET /neg-risk token_id=%s: %v", tokenID, err)
	}
	return resp.NegRisk
}

func dialFirst(urls []string) *ethclient.Client {
	for _, u := range urls {
		c, err := ethclient.Dial(u)
		if err != nil {
			continue
		}
		// 简单探活：拿一次 chainID。
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		_, err = c.ChainID(ctx)
		cancel()
		if err != nil {
			c.Close()
			continue
		}
		return c
	}
	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func verdict(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ERROR: "+format+"\n", args...)
	os.Exit(1)
}
