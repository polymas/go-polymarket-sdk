package clob

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/signing"
	"github.com/polymas/go-polymarket-sdk/types"
)

// orderedOrderV2 匹配当前 V2 Order 的 JSON 提交格式，字段顺序对齐 V2
// EIP-712 typehash 和官方 V2 SDK 的 order_to_json_v2 输出。
// orderedOrderV2 字段顺序对照 py-clob-client-v2/order_utils/model/order_data_v2.py
// 中的 order_to_json_v2() 输出顺序锁定（HMAC 签名敏感）：
//
//	salt, maker, signer, tokenId, makerAmount, takerAmount, side,
//	expiration, signatureType, timestamp, metadata, builder, signature
type orderedOrderV2 struct {
	Salt          int64  `json:"salt"`
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	TokenId       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Side          string `json:"side"`       // "BUY" / "SELL"
	Expiration    string `json:"expiration"` // unix 秒字符串；GTC 订单为 "0"
	SignatureType int    `json:"signatureType"`
	Timestamp     string `json:"timestamp"` // 毫秒整数的字符串形式
	Metadata      string `json:"metadata"`  // 0x + 64 hex
	Builder       string `json:"builder"`   // 0x + 64 hex
	Signature     string `json:"signature"` // 0x + 130 hex
}

// orderRequestV2 是单笔订单的提交载荷。字段顺序与官方 to_dict 保持一致。
type orderRequestV2 struct {
	Order     orderedOrderV2 `json:"order"`
	Owner     string         `json:"owner"`
	OrderType string         `json:"orderType"`
	DeferExec bool           `json:"deferExec"`
	PostOnly  bool           `json:"postOnly"`
}

// dropOrderbookNotFoundRegex 匹配 CLOB v2 批量端点的 top-level 错误：
//
//	HTTP 400: {"error":"the orderbook 12345 does not exist"}
//
// 捕获组取出 tokenId（十进制 uint256 字符串）。
//
// 实测（2026-05-07，POST /orders 带 3 个不同的坏 token）：CLOB 只报响应里
// 出现的**第一个**坏 token，整批 400。所以多个坏 token 必须迭代清——每轮
// 重试只能干掉一个。这里仍用 FindAll 提取，是为了防御后端将来改成一次返回
// 多个 token 的情况（届时一轮就能清干净，省重试次数）。
var dropOrderbookNotFoundRegex = regexp.MustCompile(`orderbook\s+(\d+)\s+does\s+not\s+exist`)

// postOrdersBatchV2DropMaxRetries 是 "orderbook X does not exist" 自动剥离重试
// 的最大次数。每轮把响应里报告的所有坏 token 剥掉；按当前 CLOB 行为（一次
// 只报一个），3 次重试最多能清掉 3 个不同的坏 token。批量大小上限 15，超过
// 这个数说明上游 list 大量无效，停手让人工介入。
const postOrdersBatchV2DropMaxRetries = 3

// OrderStrippedStatus 标识被 SDK 自动剥离的合成失败响应。当批量提交因
// "orderbook X does not exist" 触发剥离重试时，被剥离的订单不会出现在 CLOB
// 响应里，SDK 会在对应原始下标位置回填一条 OrderPostResponse{Status: this}，
// 以保证返回数组与入参等长同序。业务侧可用 r.Status == OrderStrippedStatus
// 做语义判断，不必字符串匹配 ErrorMsg。
const OrderStrippedStatus = "stripped"

// postOrdersBatchV2 提交当前 V2 批量订单。
//
// 重试机制（两层独立）：
//
//  1. **bad-token 重试（外层 / HTTP 400）**：CLOB 在 top-level 校验时发现某个
//     tokenId 没有对应 orderbook（已结算 / 已下架 / 错误抓取），返回 400 拒掉
//     整批。本函数自动从批内剥离该 token 的所有订单并重发，最多 3 次。
//
//  2. **negRisk 重试（内层 / HTTP 200 + per-order）**：单个订单返回
//     "invalid signature" 时改用 V2 NegRisk Exchange 重新签名+重发。
//
// 两层正交：bad-token 是请求级，negRisk 是订单级。
//
// **返回契约**：成功路径下返回的切片长度恒等于 len(orderArgsList)，且与入参
// 同序。被剥离的位置回填 OrderPostResponse{Status: OrderStrippedStatus,
// ErrorMsg: "orderbook X does not exist (stripped by SDK retry)"}，调用方可
// 直接 `for i, r := range results { ... orderArgsList[i] ... }` 按下标对齐。
// 失败路径（重试上限耗尽 / 非 bad-token 错误）保持 nil + error，不做长度补齐。
func (c *orderClientImpl) postOrdersBatchV2(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	isRetry ...bool,
) ([]types.OrderPostResponse, error) {
	return runV2BatchWithStrip(orderArgsList, orderTypes, func(a []types.OrderArgs, t []types.OrderType) ([]types.OrderPostResponse, error) {
		return c.postOrdersBatchV2Once(a, t, isRetry...)
	})
}

// runV2BatchWithStrip 实现 bad-token 自动剥离重试 + 等长结果回填。抽出来是
// 为了让纯逻辑（索引簿记、stripped 合成响应、retry 上限语义）能脱离
// orderClientImpl 单测——见 orders_v2_strip_test.go。
func runV2BatchWithStrip(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	postFn func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error),
) ([]types.OrderPostResponse, error) {
	n := len(orderArgsList)
	args := append([]types.OrderArgs(nil), orderArgsList...)
	otypes := append([]types.OrderType(nil), orderTypes...)
	// originalIndex[i] = 当前 args[i] 在原始 orderArgsList 中的下标
	originalIndex := make([]int, n)
	for i := range originalIndex {
		originalIndex[i] = i
	}
	// 被剥离订单的原始下标 → tokenID，用于回填合成 stripped 响应
	strippedAt := make(map[int]string, 0)

	buildFinal := func(resp []types.OrderPostResponse) []types.OrderPostResponse {
		final := make([]types.OrderPostResponse, n)
		for j, r := range resp {
			if j >= len(originalIndex) {
				break
			}
			final[originalIndex[j]] = r
		}
		for origIdx, tokenID := range strippedAt {
			final[origIdx] = types.OrderPostResponse{
				Status:   OrderStrippedStatus,
				ErrorMsg: fmt.Sprintf("orderbook %s does not exist (stripped by SDK retry)", tokenID),
			}
		}
		return final
	}

	for attempt := 0; ; attempt++ {
		resp, err := postFn(args, otypes)
		if err == nil {
			return buildFinal(resp), nil
		}
		if attempt >= postOrdersBatchV2DropMaxRetries {
			return nil, fmt.Errorf("V2 batch order: 已剥离 %d 个不存在的 orderbook 但批内仍有更多坏 token（达到 %d 次上限），上游 list 可能严重过期，请人工排查: %w",
				attempt, postOrdersBatchV2DropMaxRetries, err)
		}
		badTokens := extractBadOrderbookTokensFromErr(err)
		if len(badTokens) == 0 {
			// 非 "orderbook X does not exist" 的错误：直接透传，不重试
			return nil, err
		}
		// 把响应里报告的所有坏 token 一并剥离（防御 CLOB 后端改成多 token 响应）
		badSet := make(map[string]struct{}, len(badTokens))
		for _, t := range badTokens {
			badSet[t] = struct{}{}
		}
		newArgs := make([]types.OrderArgs, 0, len(args))
		newTypes := make([]types.OrderType, 0, len(otypes))
		newIdx := make([]int, 0, len(args))
		droppedPerToken := make(map[string]int, len(badTokens))
		for i, a := range args {
			if _, bad := badSet[a.TokenID]; bad {
				droppedPerToken[a.TokenID]++
				strippedAt[originalIndex[i]] = a.TokenID
				continue
			}
			newArgs = append(newArgs, a)
			newTypes = append(newTypes, otypes[i])
			newIdx = append(newIdx, originalIndex[i])
		}
		totalDropped := 0
		for _, c := range droppedPerToken {
			totalDropped += c
		}
		if totalDropped == 0 {
			// CLOB 报告的 token 不在我们这批里——无法处理，避免死循环
			return nil, fmt.Errorf("V2 batch: CLOB reports bad orderbook(s) %v but no matching order in batch: %w",
				badTokens, err)
		}
		// 日志：列出每个被剥离的 token + 笔数，便于排查上游 list 来源问题
		var summary []string
		for t, c := range droppedPerToken {
			summary = append(summary, fmt.Sprintf("%s×%d", t, c))
		}
		internal.LogError("V2 batch: 剥离不存在的 orderbook [%s]，剩余 %d 笔重试 (attempt %d/%d)",
			strings.Join(summary, ", "), len(newArgs), attempt+1, postOrdersBatchV2DropMaxRetries)
		args = newArgs
		otypes = newTypes
		originalIndex = newIdx
		if len(args) == 0 {
			// 全部被剥离：直接构造 final 即可（resp 为空切片，buildFinal
			// 会把所有原始下标都填成 stripped）
			return buildFinal(nil), nil
		}
	}
}

// extractBadOrderbookTokensFromErr 从 HTTP 400 错误里抠出 CLOB 报告的所有坏
// token。CLOB 实测一次报一个，但用 FindAllStringSubmatch 兼容未来后端把多个
// token 写在同一条响应里的情况（去重后返回）。
func extractBadOrderbookTokensFromErr(err error) []string {
	if err == nil {
		return nil
	}
	matches := dropOrderbookNotFoundRegex.FindAllStringSubmatch(err.Error(), -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		out = append(out, m[1])
	}
	return out
}

// postOrdersBatchV2Once 是 postOrdersBatchV2 的单次提交逻辑（不含 bad-token
// 重试）。负责 negRisk 内部重试 + 真正的 HTTP POST。
func (c *orderClientImpl) postOrdersBatchV2Once(
	orderArgsList []types.OrderArgs,
	orderTypes []types.OrderType,
	isRetry ...bool,
) ([]types.OrderPostResponse, error) {
	isRetryCall := len(isRetry) > 0 && isRetry[0]
	if len(orderArgsList) == 0 {
		return []types.OrderPostResponse{}, nil
	}
	if len(orderArgsList) > 15 {
		return nil, fmt.Errorf("postOrdersBatchV2: batch size cannot exceed 15, got %d", len(orderArgsList))
	}

	negRisk := false
	if isRetryCall {
		negRisk = true
	}

	requestBody := make([]orderRequestV2, 0, len(orderArgsList))

	// 非重试路径：并发预取本批中未传入、未缓存的 token 的 NegRisk 状态，把签名
	// 循环里逐个串行 GET /neg-risk 压成约 1 次往返的墙钟时间。重试路径强制 true，
	// 无需预取。
	if !isRetryCall {
		c.baseClient.prefetchNegRisk(orderArgsList)
	}

	for i, orderArgs := range orderArgsList {
		// type-3（POLY_1271）签名的 exchange 域取决于市场是否 NegRisk：
		// 非 retry 时按真实状态签对域；retry 时 negRisk 已被强制为 true，作为
		// 整批兜底（首次按真实值仍签错的极端情况下再统一改走 NegRisk Exchange
		// 重签）。真实状态优先取调用方传入的 orderArgs.NegRisk（上游多半已从
		// gamma/clob 市场数据拿到，零网络开销）；未传（nil）才现查 GET /neg-risk。
		perOrderNegRisk := negRisk
		if !isRetryCall {
			if orderArgs.NegRisk != nil {
				perOrderNegRisk = *orderArgs.NegRisk
				// 顺手把权威值喂进缓存，惠及后续 GetNegRisk / 重试路径。
				c.baseClient.negRisk[orderArgs.TokenID] = perOrderNegRisk
			} else {
				perOrderNegRisk = c.baseClient.negRiskForToken(orderArgs.TokenID)
			}
		}

		signed, err := c.createSignedOrderV2(orderArgs, orderArgs.TickSize, perOrderNegRisk, orderTypes[i])
		if err != nil {
			continue
		}

		requestBody = append(requestBody, orderRequestV2{
			Order:     signedOrderToJSONV2(signed, orderArgs.Side, orderArgs.Expiration),
			Owner:     c.baseClient.deriveCreds.Key,
			OrderType: string(orderTypes[i]),
			DeferExec: orderArgs.DeferExec,
			PostOnly:  orderArgs.PostOnly,
		})
	}

	if len(requestBody) == 0 {
		return []types.OrderPostResponse{}, fmt.Errorf("no valid orders to post")
	}

	bodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	// 匹配 Python json.dumps 的空格格式（HMAC 签名要求同一字节序列）
	bodyJSONStr := string(bodyJSON)
	bodyJSONStr = regexp.MustCompile(`":(\S)`).ReplaceAllString(bodyJSONStr, `": $1`)
	bodyJSONStr = regexp.MustCompile(`,(")`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSONStr = regexp.MustCompile(`,(\{|\[)`).ReplaceAllString(bodyJSONStr, `, $1`)
	bodyJSON = []byte(bodyJSONStr)

	requestArgs := &types.RequestArgs{
		Method:      "POST",
		RequestPath: internal.PostOrders,
	}
	headers, err := internal.CreateLevel2HeadersWithBody(c.baseClient.web3Client.GetSigner(), c.baseClient.deriveCreds, requestArgs, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create headers: %w", err)
	}

	responseBody, err := http.PostRaw(c.baseClient.baseURL, internal.PostOrders, bodyJSON, http.WithHeaders(headers))
	if err != nil {
		return nil, err
	}

	var resp []types.OrderPostResponse
	if len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &resp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
	}
	if len(resp) == 0 {
		return []types.OrderPostResponse{}, nil
	}

	// 对可能签错 exchange 域的订单执行一次 NegRisk 重签重试。
	failedOrders := make([]int, 0)
	for i, result := range resp {
		// 签名域选错（含 1271 钱包的 "invalid POLY_1271 signature: ..."）时
		// 改用 NegRisk Exchange 域重签重试，判据见 signatureRetryNeeded。
		if signatureRetryNeeded(result.ErrorMsg) {
			failedOrders = append(failedOrders, i)
		} else if result.ErrorMsg != "" {
			internal.LogError("V2 订单 %d 创建失败: %s", i+1, result.ErrorMsg)
		}
	}
	if len(failedOrders) > 0 && !isRetryCall {
		retryArgs := make([]types.OrderArgs, 0, len(failedOrders))
		retryTypes := make([]types.OrderType, 0, len(failedOrders))
		for _, idx := range failedOrders {
			retryArgs = append(retryArgs, orderArgsList[idx])
			retryTypes = append(retryTypes, orderTypes[idx])
		}
		retryResults, err := c.postOrdersBatchV2(retryArgs, retryTypes, true)
		if err != nil {
			internal.LogError("V2 重试订单失败: %v", err)
		} else {
			for j, idx := range failedOrders {
				if j < len(retryResults) && retryResults[j].ErrorMsg == "" {
					resp[idx] = retryResults[j]
				}
			}
		}
	}

	return resp, nil
}

// createSignedOrderV2 构建并签名一笔 V2 订单，返回 signing.V2SignedOrder。
func (c *orderClientImpl) createSignedOrderV2(
	orderArgs types.OrderArgs,
	tickSize types.TickSize,
	negRisk bool,
	_ types.OrderType, // V2 没有 expiration 字段，orderType 只影响提交时的 orderType 字段
) (*signing.V2SignedOrder, error) {
	tickSizeFloat, err := parseRequiredTickSize(tickSize)
	if err != nil {
		return nil, fmt.Errorf("invalid tick size: %w", err)
	}
	if orderArgs.Price < tickSizeFloat || orderArgs.Price > 1.0-tickSizeFloat {
		return nil, fmt.Errorf("price (%.6f) out of range [%.6f, %.6f]", orderArgs.Price, tickSizeFloat, 1.0-tickSizeFloat)
	}

	makerAmount, takerAmount, err := c.calculateOrderAmounts(orderArgs.Side, orderArgs.Size, orderArgs.Price, tickSize)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate amounts: %w", err)
	}

	baseAddr := string(c.baseClient.web3Client.GetBaseAddress())
	makerAddr := baseAddr
	signerAddr := baseAddr
	switch c.baseClient.signatureType {
	case types.ProxySignatureType, types.SafeSignatureType:
		// Proxy/Safe：maker = proxy（资金来源），signer = EOA（API key 持有者）
		makerAddr = string(c.baseClient.proxyAddress)
	case types.CWIASignatureType:
		// V2 POLY_1271 (=3) 语义：合约钱包通过 EIP-1271 验证签名，
		// 因此 signer 字段必须 = funder/proxy 地址，不能是 EOA。
		// 对照 py-clob-client-v2/order_builder/builder.py 的 _v2_order_signer：
		//   if signature_type == POLY_1271: return funder
		// 不这么做 CLOB 会报 "the order signer address has to be the
		// address of the API KEY"（v2 后端对 1271 类型走另一套校验）。
		makerAddr = string(c.baseClient.proxyAddress)
		signerAddr = string(c.baseClient.proxyAddress)
	}

	var side signing.V2OrderSide
	if orderArgs.Side == types.OrderSideBUY {
		side = signing.V2SideBUY
	} else {
		side = signing.V2SideSELL
	}

	exchangeAddrHex := internal.PolygonExchangeV2
	if negRisk {
		exchangeAddrHex = internal.PolygonNegRiskExchangeV2
	}

	chainID := big.NewInt(int64(c.baseClient.web3Client.GetChainID()))
	return signing.BuildSignedV2Order(
		c.baseClient.web3Client.GetPrivateKey(),
		&signing.V2OrderData{
			Maker:         makerAddr,
			Signer:        signerAddr,
			TokenID:       orderArgs.TokenID,
			MakerAmount:   makerAmount.String(),
			TakerAmount:   takerAmount.String(),
			Side:          side,
			SignatureType: c.baseClient.signatureType,
			// TimestampMS=0 → BuildSignedV2Order 自动填当前时间
		},
		chainID,
		common.HexToAddress(exchangeAddrHex),
	)
}

// signedOrderToJSONV2 把 V2SignedOrder 转换成请求体 JSON 结构。
//
// expiration 取自 OrderArgs.Expiration（unix 秒）：0 表示 GTC（不过期），
// 非 0 表示 GTD 订单的截止时间。注意：expiration 仅出现在 wire body 里，
// 不是 V2 EIP-712 签名结构的一部分（见 docs.polymarket.com 的 post-multiple-orders
// 说明："expiration remains in the POST /order wire body for GTD/order-expiry
// handling, but it is not part of the CLOB V2 EIP-712 signed order struct"）。
func signedOrderToJSONV2(s *signing.V2SignedOrder, side types.OrderSide, expiration int64) orderedOrderV2 {
	return orderedOrderV2{
		Salt:          s.Salt.Int64(),
		Maker:         s.Maker.Hex(),
		Signer:        s.Signer.Hex(),
		TokenId:       s.TokenID.String(),
		MakerAmount:   s.MakerAmount.String(),
		TakerAmount:   s.TakerAmount.String(),
		Side:          string(side),
		Expiration:    strconv.FormatInt(expiration, 10),
		SignatureType: int(s.SignatureType),
		Timestamp:     s.TimestampMS.String(),
		Metadata:      "0x" + hex.EncodeToString(s.Metadata[:]),
		Builder:       "0x" + hex.EncodeToString(s.Builder[:]),
		Signature:     "0x" + hex.EncodeToString(s.Signature),
	}
}
