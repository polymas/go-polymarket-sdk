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

// orderedOrderV2 匹配 V2 Order 的 JSON 提交格式，字段顺序对齐 V2 EIP-712 typehash。
// 注意：服务端确切字段名和 salt/signatureType 的数值/字符串选择，官方 V2 POST /order 文档
// 未正式公开。下面按 V1 惯例推断（salt/signatureType 为整数，其它数值量为十进制字符串）。
// 联调时如遇 422，先看这里的键名。
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
	Side          string `json:"side"`         // "BUY" / "SELL"
	Expiration    string `json:"expiration"`   // unix 秒字符串；GTC 订单为 "0"
	SignatureType int    `json:"signatureType"`
	Timestamp     string `json:"timestamp"`    // 毫秒整数的字符串形式
	Metadata      string `json:"metadata"`     // 0x + 64 hex
	Builder       string `json:"builder"`      // 0x + 64 hex
	Signature     string `json:"signature"`    // 0x + 130 hex
}

// orderRequestV2 是单笔订单的提交载荷。字段顺序与官方 to_dict 保持一致。
type orderRequestV2 struct {
	Order     orderedOrderV2 `json:"order"`
	Owner     string         `json:"owner"`
	OrderType string         `json:"orderType"`
	DeferExec bool           `json:"deferExec"`
	PostOnly  bool           `json:"postOnly"`
}

// postOrdersBatchV2 对应 V1 的 postOrdersBatch，走 V2 签名路径。
// 沿用 V1 的 negRisk 重试模式：先以 V2 标准 Exchange 为 verifyingContract 签，
// 若返回 "invalid signature" 则改用 V2 NegRisk Exchange 重试。
func (c *orderClientImpl) postOrdersBatchV2(
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

	const defaultTickSize types.TickSize = "0.001"
	negRisk := false
	if isRetryCall {
		negRisk = true
	}

	requestBody := make([]orderRequestV2, 0, len(orderArgsList))

	for i, orderArgs := range orderArgsList {
		if orderArgs.Size < 5.0 {
			orderArgs.Size = 5.0
		}

		signed, err := c.createSignedOrderV2(orderArgs, defaultTickSize, negRisk, orderTypes[i])
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

	// V1 相同的 negRisk 重试流程
	failedOrders := make([]int, 0)
	for i, result := range resp {
		if result.ErrorMsg != "" && strings.Contains(result.ErrorMsg, "invalid signature") {
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
	tickSizeFloat, err := strconv.ParseFloat(string(tickSize), 64)
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
		// V1 风格：maker = proxy（资金来源），signer = EOA（API key 持有者）
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
