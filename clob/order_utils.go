package clob

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/polymas/go-polymarket-sdk/types"
)

// isTickSizeSmaller 检查 tickSize a 是否小于 tickSize b
// 对应 Python 的 is_tick_size_smaller 函数
func isTickSizeSmaller(a types.TickSize, b types.TickSize) bool {
	aFloat, errA := strconv.ParseFloat(string(a), 64)
	bFloat, errB := strconv.ParseFloat(string(b), 64)
	if errA != nil || errB != nil {
		// 如果解析失败，使用字符串比较（fallback）
		return string(a) < string(b)
	}
	return aFloat < bFloat
}

// calculateOrderAmounts calculates maker and taker amounts based on side, size, price, and tick size
func (c *orderClientImpl) calculateOrderAmounts(
	side types.OrderSide,
	size float64,
	price float64,
	tickSize types.TickSize,
) (*big.Int, *big.Int, error) {
	// Parse tick size
	tickSizeFloat, err := strconv.ParseFloat(string(tickSize), 64)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid tick size: %w", err)
	}

	// Round price to tick size using round_normal (ROUND_HALF_UP) matching Python
	roundedPrice := c.roundNormal(price, tickSizeFloat)

	// Convert to token decimals (1e6) - matching Python's to_token_decimals
	// Python: to_token_decimals(x) = int(Decimal(str(x)) * Decimal(10**6).quantize(exp=Decimal(1), rounding=ROUND_HALF_UP))
	toTokenDecimals := func(val float64) *big.Int {
		// Multiply by 1e6 and convert to big.Int (ROUND_HALF_UP)
		valBig := new(big.Float).SetFloat64(val)
		multiplier := new(big.Float).SetFloat64(1e6)
		result := new(big.Float).Mul(valBig, multiplier)
		// Round to nearest integer (ROUND_HALF_UP)
		intResult, _ := result.Int(nil)
		// Check if we need to round up (if fractional part >= 0.5)
		frac := new(big.Float).Sub(result, new(big.Float).SetInt(intResult))
		if frac.Cmp(new(big.Float).SetFloat64(0.5)) >= 0 {
			intResult.Add(intResult, big.NewInt(1))
		}
		return intResult
	}

	// Round down size to 2 decimal places (matching Python's round_config.size = 2)
	roundedSize := c.roundDown(size, 2)

	if side == types.OrderSideBUY {
		// BUY: taker_amount = size, maker_amount = size * price
		takerAmount := roundedSize
		makerAmount := takerAmount * roundedPrice

		// Round maker amount following Python logic:
		// 1. If decimal places > round_config.amount (6), try round_up to (amount + 4) = 10
		// 2. If still > amount, round_down to amount = 6
		makerAmount = c.roundMakerAmount(makerAmount, tickSizeFloat)

		return toTokenDecimals(makerAmount), toTokenDecimals(takerAmount), nil
	} else {
		// SELL: maker_amount = size, taker_amount = size * price
		makerAmount := roundedSize
		takerAmount := makerAmount * roundedPrice

		// Round taker amount following Python logic:
		// 1. If decimal places > round_config.amount (6), try round_up to (amount + 4) = 10
		// 2. If still > amount, round_down to amount = 6
		takerAmount = c.roundMakerAmount(takerAmount, tickSizeFloat)

		return toTokenDecimals(makerAmount), toTokenDecimals(takerAmount), nil
	}
}

// roundNormal rounds a price to tick size using ROUND_HALF_UP (matching Python's round_normal)
// Python: round_normal(x, sig_digits) uses Decimal.quantize(exp=Decimal(1).scaleb(-sig_digits), rounding=ROUND_HALF_UP)
func (c *orderClientImpl) roundNormal(price float64, tickSize float64) float64 {
	if tickSize <= 0 {
		return price
	}
	// Calculate number of decimal places from tick size
	// For tick size 0.0001, we need 4 decimal places
	decimals := c.getDecimalPlacesFromTickSize(tickSize)

	// Round using ROUND_HALF_UP (standard math.Round)
	multiplier := 1.0
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	rounded := float64(int64(price*multiplier+0.5)) / multiplier
	return rounded
}

// getDecimalPlacesFromTickSize calculates decimal places from tick size
// tick_size 0.1 -> 1 decimal, 0.01 -> 2 decimals, 0.001 -> 3 decimals, 0.0001 -> 4 decimals
func (c *orderClientImpl) getDecimalPlacesFromTickSize(tickSize float64) int {
	if tickSize >= 0.1 {
		return 1
	} else if tickSize >= 0.01 {
		return 2
	} else if tickSize >= 0.001 {
		return 3
	} else {
		return 4 // Default for 0.0001
	}
}

// roundDown rounds down to specified decimal places
func (c *orderClientImpl) roundDown(val float64, decimals int) float64 {
	multiplier := 1.0
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	return float64(int64(val*multiplier)) / multiplier
}

// roundMakerAmount rounds maker/taker amount following Python's logic:
// 1. Get round_config.amount based on tick size (6 for 0.0001, 5 for 0.001, etc.)
// 2. If decimal places > amount, try round_up to (amount + 4)
// 3. If still > amount, round_down to amount
func (c *orderClientImpl) roundMakerAmount(amount float64, tickSize float64) float64 {
	// Determine round_config.amount from tick size (matching Python ROUNDING_CONFIG)
	var amountDecimals int
	if tickSize >= 0.1 {
		amountDecimals = 3
	} else if tickSize >= 0.01 {
		amountDecimals = 4
	} else if tickSize >= 0.001 {
		amountDecimals = 5
	} else {
		amountDecimals = 6 // Default for 0.0001
	}

	// Count decimal places
	decimalPlaces := c.countDecimalPlaces(amount)

	// If decimal places <= amount, no rounding needed
	if decimalPlaces <= amountDecimals {
		return amount
	}

	// Try round_up to (amount + 4) first
	roundedUp := c.roundUp(amount, amountDecimals+4)
	decimalPlacesAfterRoundUp := c.countDecimalPlaces(roundedUp)

	// If still > amount, round_down to amount
	if decimalPlacesAfterRoundUp > amountDecimals {
		return c.roundDown(amount, amountDecimals)
	}

	return roundedUp
}

// countDecimalPlaces counts the number of decimal places in a float
func (c *orderClientImpl) countDecimalPlaces(val float64) int {
	// Convert to string to count decimal places
	str := fmt.Sprintf("%.10f", val)
	str = strings.TrimRight(str, "0")
	str = strings.TrimRight(str, ".")
	if !strings.Contains(str, ".") {
		return 0
	}
	parts := strings.Split(str, ".")
	if len(parts) != 2 {
		return 0
	}
	return len(parts[1])
}

// roundUp rounds up to specified decimal places
func (c *orderClientImpl) roundUp(val float64, decimals int) float64 {
	multiplier := 1.0
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	// Round up: add a small epsilon before truncating
	epsilon := 0.5 / multiplier
	return float64(int64((val+epsilon)*multiplier)) / multiplier
}
