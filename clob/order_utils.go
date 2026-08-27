package clob

import (
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/polymas/go-polymarket-sdk/types"
)

const (
	orderSizeDecimals   = 2
	defaultMinOrderSize = 5.0
)

// orderRoundingConfig 与官方 V2 order builder 的 ROUNDING_CONFIG 对齐。
// limit order 的 amount 精度恒等于 price 精度 + size 的两位精度。
type orderRoundingConfig struct {
	priceDecimals  int
	amountDecimals int
	priceScale     int64
	tickUnits      int64
}

func roundingConfigForTickSize(tickSize types.TickSize) (orderRoundingConfig, bool) {
	switch tickSize {
	case types.TickSize0_1:
		return orderRoundingConfig{priceDecimals: 1, amountDecimals: 3, priceScale: 10, tickUnits: 1}, true
	case types.TickSize0_01:
		return orderRoundingConfig{priceDecimals: 2, amountDecimals: 4, priceScale: 100, tickUnits: 1}, true
	case types.TickSize0_005:
		return orderRoundingConfig{priceDecimals: 3, amountDecimals: 5, priceScale: 1_000, tickUnits: 5}, true
	case types.TickSize0_0025:
		return orderRoundingConfig{priceDecimals: 4, amountDecimals: 6, priceScale: 10_000, tickUnits: 25}, true
	case types.TickSize0_001:
		return orderRoundingConfig{priceDecimals: 3, amountDecimals: 5, priceScale: 1_000, tickUnits: 1}, true
	case types.TickSize0_0001:
		return orderRoundingConfig{priceDecimals: 4, amountDecimals: 6, priceScale: 10_000, tickUnits: 1}, true
	default:
		return orderRoundingConfig{}, false
	}
}

// parseRequiredTickSize 解析业务层显式传入的 tick size。
// 下单路径只消费该值，不在这里或调用方上层隐式请求 /tick-size。
func parseRequiredTickSize(tickSize types.TickSize) (float64, error) {
	raw := string(tickSize)
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("tick_size is required")
	}
	config, ok := roundingConfigForTickSize(tickSize)
	if !ok {
		return 0, fmt.Errorf("invalid tick_size %q", raw)
	}
	return float64(config.tickUnits) / float64(config.priceScale), nil
}

// priceUnitsOnTick 将价格归一为 tick 对应的小数整数，并校验其严格落在网格上。
// tolerance 只吸收 float64 的二进制表示误差，不会把真实的非网格价格自动舍入。
func priceUnitsOnTick(price float64, tickSize types.TickSize) (int64, orderRoundingConfig, error) {
	config, ok := roundingConfigForTickSize(tickSize)
	if !ok {
		return 0, orderRoundingConfig{}, fmt.Errorf("invalid tick_size %q", tickSize)
	}
	if math.IsNaN(price) || math.IsInf(price, 0) {
		return 0, orderRoundingConfig{}, fmt.Errorf("price must be finite")
	}

	scaled := price * float64(config.priceScale)
	nearest := math.Round(scaled)
	const floatTolerance = 1e-8
	if math.Abs(scaled-nearest) > floatTolerance {
		return 0, orderRoundingConfig{}, fmt.Errorf("price %g is not aligned to tick_size %s", price, tickSize)
	}
	units := int64(nearest)
	if units%config.tickUnits != 0 {
		return 0, orderRoundingConfig{}, fmt.Errorf("price %g is not aligned to tick_size %s", price, tickSize)
	}
	return units, config, nil
}

// validateOrderTickSizes 对一批订单逐笔校验业务层传入的 tick size 和价格范围。
// 这是纯本地校验，不读取缓存，也不会发起任何市场数据请求。
func validateOrderTickSizes(orderArgsList []types.OrderArgs) error {
	for i, orderArgs := range orderArgsList {
		tickSize, err := parseRequiredTickSize(orderArgs.TickSize)
		if err != nil {
			return fmt.Errorf("订单 %d token=%s: %w", i+1, orderArgs.TokenID, err)
		}
		if math.IsNaN(orderArgs.Price) || math.IsInf(orderArgs.Price, 0) ||
			orderArgs.Price < tickSize || orderArgs.Price > 1.0-tickSize {
			return fmt.Errorf("订单 %d 价格无效: price=%g 必须在范围 [%g, %g] 内（tick_size=%s）",
				i+1, orderArgs.Price, tickSize, 1.0-tickSize, orderArgs.TickSize)
		}
		if _, _, err := priceUnitsOnTick(orderArgs.Price, orderArgs.TickSize); err != nil {
			return fmt.Errorf("订单 %d token=%s: %w", i+1, orderArgs.TokenID, err)
		}
	}
	return nil
}

// validateOrderSizes 保留 SDK 原有的默认最小数量 5，但不再
// 静默改写调用方的订单。该校验为纯本地操作，不请求市场配置。
func validateOrderSizes(orderArgsList []types.OrderArgs) error {
	for i, orderArgs := range orderArgsList {
		if math.IsNaN(orderArgs.Size) || math.IsInf(orderArgs.Size, 0) || orderArgs.Size < defaultMinOrderSize {
			return fmt.Errorf("订单 %d token=%s: size=%g 无效或小于默认最小值 %g",
				i+1, orderArgs.TokenID, orderArgs.Size, defaultMinOrderSize)
		}
	}
	return nil
}

// validateOrderTokenIDs 在切批和签名前校验 tokenID，避免后续子批构造
// EIP-712 订单时才发现错误，造成前面子批已提交的局面。
func validateOrderTokenIDs(orderArgsList []types.OrderArgs) error {
	for i, orderArgs := range orderArgsList {
		tokenID, ok := new(big.Int).SetString(orderArgs.TokenID, 10)
		if !ok || tokenID.Sign() <= 0 || tokenID.BitLen() > 256 {
			return fmt.Errorf("订单 %d token=%s: tokenID 必须是有效的 uint256 十进制整数",
				i+1, orderArgs.TokenID)
		}
	}
	return nil
}

// calculateOrderAmounts calculates maker and taker amounts based on side, size, price, and tick size
func (c *orderClientImpl) calculateOrderAmounts(
	side types.OrderSide,
	size float64,
	price float64,
	tickSize types.TickSize,
) (*big.Int, *big.Int, error) {
	priceUnits, config, err := priceUnitsOnTick(price, tickSize)
	if err != nil {
		return nil, nil, err
	}
	if math.IsNaN(size) || math.IsInf(size, 0) || size <= 0 {
		return nil, nil, fmt.Errorf("size must be finite and positive")
	}

	// 官方 builder 对 size 向下保留两位。加上的微小 tolerance 只吸收
	// 12.34*100 这类 float64 表示误差，不会把第三位小数进位。
	sizeHundredths := int64(math.Floor(size*100 + 1e-9))
	if sizeHundredths <= 0 {
		return nil, nil, fmt.Errorf("size is too small after rounding to %d decimals", orderSizeDecimals)
	}

	// size 最多两位、price 最多四位，因此乘积最多六位，正好能无损转换为
	// CLOB 使用的 1e6 token units。使用 big.Int 避免金额乘法溢出。
	sizeAmount := new(big.Int).Mul(big.NewInt(sizeHundredths), big.NewInt(10_000))
	quoteFactor := int64(1_000_000) / (100 * config.priceScale)
	quoteAmount := new(big.Int).Mul(big.NewInt(sizeHundredths), big.NewInt(priceUnits))
	quoteAmount.Mul(quoteAmount, big.NewInt(quoteFactor))

	if side == types.OrderSideBUY {
		return quoteAmount, sizeAmount, nil
	}
	return sizeAmount, quoteAmount, nil
}
