package clob

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/polymas/go-polymarket-sdk/internal"
	http "github.com/polymas/go-polymarket-sdk/internal/transport"
	"github.com/polymas/go-polymarket-sdk/types"
)

// InsufficientLiquidityError 表示当前订单簿无法完整满足 FOK 市价单。
// BUY 的 Requested/Available 单位是 pUSD，SELL 的单位是 shares。
type InsufficientLiquidityError struct {
	Side      types.OrderSide
	Requested float64
	Available float64
}

func (e *InsufficientLiquidityError) Error() string {
	return fmt.Sprintf("insufficient %s liquidity: requested=%g available=%g", e.Side, e.Requested, e.Available)
}

type preparedMarketOrder struct {
	orderArgs   types.OrderArgs
	orderType   types.OrderType
	makerAmount *big.Int
	takerAmount *big.Int
}

// CalculateMarketPrice 按官方 CLOB SDK 的规则从当前订单簿估算市价单价格。
func (c *marketDataClientImpl) CalculateMarketPrice(
	tokenID string,
	side types.OrderSide,
	amount float64,
	orderType types.OrderType,
) (float64, error) {
	return c.CalculateMarketPriceContext(context.Background(), tokenID, side, amount, orderType)
}

func (c *marketDataClientImpl) CalculateMarketPriceContext(ctx context.Context, tokenID string, side types.OrderSide, amount float64, orderType types.OrderType) (float64, error) {
	return calculateMarketPriceAtURLContext(ctx, c.baseURL, tokenID, side, amount, orderType)
}

// CalculateMarketPrice 是无鉴权的只读实现。
func (c *readonlyMarketDataClientImpl) CalculateMarketPrice(
	tokenID string,
	side types.OrderSide,
	amount float64,
	orderType types.OrderType,
) (float64, error) {
	return c.CalculateMarketPriceContext(context.Background(), tokenID, side, amount, orderType)
}

func (c *readonlyMarketDataClientImpl) CalculateMarketPriceContext(ctx context.Context, tokenID string, side types.OrderSide, amount float64, orderType types.OrderType) (float64, error) {
	return calculateMarketPriceAtURLContext(ctx, c.baseURL, tokenID, side, amount, orderType)
}

func calculateMarketPriceAtURL(
	baseURL string,
	tokenID string,
	side types.OrderSide,
	amount float64,
	orderType types.OrderType,
) (float64, error) {
	return calculateMarketPriceAtURLContext(context.Background(), baseURL, tokenID, side, amount, orderType)
}

func calculateMarketPriceAtURLContext(ctx context.Context, baseURL string, tokenID string, side types.OrderSide, amount float64, orderType types.OrderType) (float64, error) {
	if err := validateOrderTokenIDs([]types.OrderArgs{{TokenID: tokenID}}); err != nil {
		return 0, err
	}
	book, err := http.GetContext[types.OrderBookSummary](ctx, baseURL, internal.GetOrderBook, map[string]string{"token_id": tokenID}, http.WithService("clob"))
	if err != nil {
		return 0, fmt.Errorf("get order book for market price: %w", err)
	}
	if book.TokenID != "" && book.TokenID != tokenID {
		return 0, fmt.Errorf("order book token mismatch: got %s, want %s", book.TokenID, tokenID)
	}
	return calculateMarketPriceFromBook(book, side, amount, orderType)
}

func calculateMarketPriceFromBook(
	book *types.OrderBookSummary,
	side types.OrderSide,
	amount float64,
	orderType types.OrderType,
) (float64, error) {
	if book == nil {
		return 0, fmt.Errorf("order book is nil")
	}
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
		return 0, fmt.Errorf("market order amount must be finite and positive")
	}
	orderType, err := normalizeMarketOrderType(orderType)
	if err != nil {
		return 0, err
	}

	var levels []types.OrderLevel
	switch side {
	case types.OrderSideBUY:
		levels = book.Asks
	case types.OrderSideSELL:
		levels = book.Bids
	default:
		return 0, fmt.Errorf("market order side must be BUY or SELL")
	}
	if len(levels) == 0 {
		return 0, &InsufficientLiquidityError{Side: side, Requested: amount}
	}

	available := 0.0
	for i := len(levels) - 1; i >= 0; i-- {
		price, size, parseErr := parseOrderLevel(levels[i])
		if parseErr != nil {
			return 0, fmt.Errorf("invalid order book level %d: %w", i, parseErr)
		}
		if side == types.OrderSideBUY {
			available += price * size
		} else {
			available += size
		}
		if available+1e-12 >= amount {
			return price, nil
		}
	}

	if orderType == types.OrderTypeFOK {
		return 0, &InsufficientLiquidityError{Side: side, Requested: amount, Available: available}
	}
	// 官方 FAK 规则：深度不足时使用订单簿最差一档的价格，
	// 能成交多少就成交多少，剩余立即取消。
	price, _, err := parseOrderLevel(levels[0])
	return price, err
}

func parseOrderLevel(level types.OrderLevel) (float64, float64, error) {
	price, err := level.Price.Float64()
	if err != nil {
		return 0, 0, err
	}
	size, err := level.Size.Float64()
	if err != nil {
		return 0, 0, err
	}
	if !isFinitePositive(price) || !isFinitePositive(size) {
		return 0, 0, fmt.Errorf("price and size must be finite and positive")
	}
	return price, size, nil
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func normalizeMarketOrderType(orderType types.OrderType) (types.OrderType, error) {
	if orderType == "" {
		return types.OrderTypeFOK, nil
	}
	if orderType != types.OrderTypeFOK && orderType != types.OrderTypeFAK {
		return "", fmt.Errorf("market order type must be FOK or FAK, got %q", orderType)
	}
	return orderType, nil
}

func (c *orderClientImpl) prepareMarketOrder(args types.MarketOrderArgs) (*preparedMarketOrder, error) {
	return c.prepareMarketOrderContext(context.Background(), args)
}

func (c *orderClientImpl) prepareMarketOrderContext(ctx context.Context, args types.MarketOrderArgs) (*preparedMarketOrder, error) {
	if err := validateOrderTokenIDs([]types.OrderArgs{{TokenID: args.TokenID}}); err != nil {
		return nil, err
	}
	if _, err := parseRequiredTickSize(args.TickSize); err != nil {
		return nil, err
	}
	orderType, err := normalizeMarketOrderType(args.OrderType)
	if err != nil {
		return nil, err
	}

	var amount, protectedPrice float64
	protected := false
	switch args.Side {
	case types.OrderSideBUY:
		if !isFinitePositive(args.Amount) || args.Shares != 0 {
			return nil, fmt.Errorf("BUY market order requires positive Amount and zero Shares")
		}
		if args.MinPrice != 0 {
			return nil, fmt.Errorf("MinPrice is only valid for SELL market orders")
		}
		amount = args.Amount
		protectedPrice = args.MaxPrice
		protected = args.MaxPrice != 0
	case types.OrderSideSELL:
		if !isFinitePositive(args.Shares) || args.Amount != 0 {
			return nil, fmt.Errorf("SELL market order requires positive Shares and zero Amount")
		}
		if args.MaxPrice != 0 {
			return nil, fmt.Errorf("MaxPrice is only valid for BUY market orders")
		}
		amount = args.Shares
		protectedPrice = args.MinPrice
		protected = args.MinPrice != 0
	default:
		return nil, fmt.Errorf("market order side must be BUY or SELL")
	}

	price := protectedPrice
	negRisk := args.NegRisk
	if protected {
		if _, _, err := priceUnitsOnTick(price, args.TickSize); err != nil {
			return nil, fmt.Errorf("invalid protected market price: %w", err)
		}
		tick, _ := parseRequiredTickSize(args.TickSize)
		if price < tick || price > 1-tick {
			return nil, fmt.Errorf("protected market price %g must be in [%g, %g]", price, tick, 1-tick)
		}
	} else {
		book, getErr := http.GetContext[types.OrderBookSummary](ctx, c.baseURL, internal.GetOrderBook, map[string]string{"token_id": args.TokenID}, http.WithService("clob"))
		if getErr != nil {
			return nil, fmt.Errorf("get order book for market order: %w", getErr)
		}
		if book.TokenID != "" && book.TokenID != args.TokenID {
			return nil, fmt.Errorf("order book token mismatch: got %s, want %s", book.TokenID, args.TokenID)
		}
		if book.TickSize != args.TickSize {
			return nil, fmt.Errorf("stale tick_size: supplied=%s current=%s", args.TickSize, book.TickSize)
		}
		price, err = calculateMarketPriceFromBook(book, args.Side, amount, orderType)
		if err != nil {
			return nil, err
		}
		if negRisk == nil {
			bookNegRisk := book.NegRisk
			negRisk = &bookNegRisk
		}
	}

	makerAmount, takerAmount, err := calculateMarketOrderAmounts(args.Side, amount, price, args.TickSize, protected)
	if err != nil {
		return nil, err
	}
	shareAmount := takerAmount
	if args.Side == types.OrderSideSELL {
		shareAmount = makerAmount
	}
	if shareAmount.Cmp(big.NewInt(int64(defaultMinOrderSize*1_000_000))) < 0 {
		return nil, fmt.Errorf("market order shares=%s are below default minimum %g", tokenUnitsString(shareAmount), defaultMinOrderSize)
	}

	return &preparedMarketOrder{
		orderArgs: types.OrderArgs{
			TokenID:   args.TokenID,
			Price:     price,
			Size:      amount,
			Side:      args.Side,
			TickSize:  args.TickSize,
			DeferExec: args.DeferExec,
			NegRisk:   negRisk,
		},
		orderType:   orderType,
		makerAmount: makerAmount,
		takerAmount: takerAmount,
	}, nil
}

func calculateMarketOrderAmounts(
	side types.OrderSide,
	amount float64,
	price float64,
	tickSize types.TickSize,
	protected bool,
) (*big.Int, *big.Int, error) {
	priceUnits, config, err := priceUnitsOnTick(price, tickSize)
	if err != nil {
		return nil, nil, err
	}
	if !isFinitePositive(amount) {
		return nil, nil, fmt.Errorf("market order amount must be finite and positive")
	}
	amountHundredths := int64(math.Floor(amount*100 + 1e-9))
	if amountHundredths <= 0 {
		return nil, nil, fmt.Errorf("market order amount is too small after rounding to 2 decimals")
	}
	makerAmount := new(big.Int).Mul(big.NewInt(amountHundredths), big.NewInt(10_000))

	// taker 精度使用官方 ROUNDING_CONFIG：tick=0.1/0.01/0.001/0.0001
	// 分别对应 3/4/5/6 位（0.005/0.0025 分别为 5/6 位）。
	takerDecimals := config.amountDecimals
	quantum := int64(1)
	for i := 0; i < 6-takerDecimals; i++ {
		quantum *= 10
	}

	var numerator, denominator *big.Int
	switch side {
	case types.OrderSideBUY:
		numerator = new(big.Int).Mul(new(big.Int).Set(makerAmount), big.NewInt(config.priceScale))
		denominator = big.NewInt(priceUnits)
	case types.OrderSideSELL:
		numerator = new(big.Int).Mul(new(big.Int).Set(makerAmount), big.NewInt(priceUnits))
		denominator = big.NewInt(config.priceScale)
	default:
		return nil, nil, fmt.Errorf("market order side must be BUY or SELL")
	}

	coarseDenominator := new(big.Int).Mul(denominator, big.NewInt(quantum))
	coarse, remainder := new(big.Int), new(big.Int)
	coarse.QuoRem(numerator, coarseDenominator, remainder)
	if protected && remainder.Sign() > 0 {
		coarse.Add(coarse, big.NewInt(1))
	}
	takerAmount := coarse.Mul(coarse, big.NewInt(quantum))
	if takerAmount.Sign() <= 0 {
		return nil, nil, fmt.Errorf("market order taker amount rounds to zero")
	}
	return makerAmount, takerAmount, nil
}

func tokenUnitsString(amount *big.Int) string {
	whole, fraction := new(big.Int), new(big.Int)
	whole.QuoRem(amount, big.NewInt(1_000_000), fraction)
	return fmt.Sprintf("%s.%06d", whole, fraction.Int64())
}

func (c *orderClientImpl) CreateAndPostMarketOrder(args types.MarketOrderArgs) (*types.OrderPostResponse, error) {
	return c.CreateAndPostMarketOrderContext(context.Background(), args)
}

func (c *orderClientImpl) CreateAndPostMarketOrderContext(ctx context.Context, args types.MarketOrderArgs) (*types.OrderPostResponse, error) {
	return c.createAndPostMarketOrderWithModeContext(ctx, args, true)
}

func (c *orderClientImpl) CreateAndPostMarketOrderInstant(args types.MarketOrderArgs) (*types.OrderPostResponse, error) {
	return c.CreateAndPostMarketOrderInstantContext(context.Background(), args)
}

func (c *orderClientImpl) CreateAndPostMarketOrderInstantContext(ctx context.Context, args types.MarketOrderArgs) (*types.OrderPostResponse, error) {
	return c.createAndPostMarketOrderWithModeContext(ctx, args, false)
}

func (c *orderClientImpl) CreateAndPostMarketOrderAndWait(
	ctx context.Context,
	args types.MarketOrderArgs,
) (*types.OrderPostResponse, error) {
	response, submitErr := c.CreateAndPostMarketOrderInstantContext(ctx, args)
	if response == nil {
		return nil, submitErr
	}
	awaited, awaitErr := c.AwaitOrderResult(ctx, *response)
	if awaitErr != nil {
		return awaited, errors.Join(submitErr, awaitErr)
	}
	var ambiguousErr *batchPostError
	if submitErr != nil && errors.As(submitErr, &ambiguousErr) && awaited != nil && awaited.Accepted() {
		submitErr = nil
	}
	return awaited, submitErr
}

func (c *orderClientImpl) createAndPostMarketOrderWithMode(
	args types.MarketOrderArgs,
	waitForResult bool,
) (*types.OrderPostResponse, error) {
	return c.createAndPostMarketOrderWithModeContext(context.Background(), args, waitForResult)
}

func (c *orderClientImpl) createAndPostMarketOrderWithModeContext(ctx context.Context, args types.MarketOrderArgs, waitForResult bool) (*types.OrderPostResponse, error) {
	prepared, err := c.prepareMarketOrderContext(ctx, args)
	if err != nil {
		return nil, err
	}
	orderArgs := []types.OrderArgs{prepared.orderArgs}
	orderTypes := []types.OrderType{prepared.orderType}
	overrides := []orderAmountOverride{{maker: prepared.makerAmount, taker: prepared.takerAmount}}
	results, postErr := resolveV2BatchAttempt(orderArgs, orderTypes, func(a []types.OrderArgs, t []types.OrderType) ([]types.OrderPostResponse, error) {
		return c.postOrdersBatchV2OnceWithAmountsContext(ctx, a, t, waitForResult, overrides, false)
	})
	return firstOrderResponse(results, postErr)
}
