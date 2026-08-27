package clob

import (
	"math"
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestParseRequiredTickSize(t *testing.T) {
	for _, value := range []types.TickSize{
		types.TickSize0_1,
		types.TickSize0_01,
		types.TickSize0_005,
		types.TickSize0_0025,
		types.TickSize0_001,
		types.TickSize0_0001,
	} {
		t.Run(string(value), func(t *testing.T) {
			got, err := parseRequiredTickSize(value)
			if err != nil {
				t.Fatalf("parseRequiredTickSize(%q): %v", value, err)
			}
			if got <= 0 {
				t.Fatalf("parseRequiredTickSize(%q) = %v", value, got)
			}
		})
	}
}

func TestParseRequiredTickSizeRejectsMissingAndInvalid(t *testing.T) {
	tests := []struct {
		name    string
		value   types.TickSize
		wantErr string
	}{
		{name: "missing", wantErr: "required"},
		{name: "spaces", value: "   ", wantErr: "required"},
		{name: "padded enum", value: " 0.01 ", wantErr: "invalid"},
		{name: "not a number", value: "abc", wantErr: "invalid"},
		{name: "zero", value: "0", wantErr: "invalid"},
		{name: "negative", value: "-0.01", wantErr: "invalid"},
		{name: "too large", value: "0.5", wantErr: "invalid"},
		{name: "nan", value: "NaN", wantErr: "invalid"},
		{name: "infinity", value: "+Inf", wantErr: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRequiredTickSize(tt.value)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("parseRequiredTickSize(%q) error = %v, want %q", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOrderTickSizesSupportsPerOrderValues(t *testing.T) {
	orders := []types.OrderArgs{
		{TokenID: "regular", Price: 0.01, TickSize: types.TickSize0_01},
		{TokenID: "fine", Price: 0.001, TickSize: types.TickSize0_001},
		{TokenID: "quarter", Price: 0.0025, TickSize: types.TickSize0_0025},
	}
	if err := validateOrderTickSizes(orders); err != nil {
		t.Fatalf("valid per-order tick sizes rejected: %v", err)
	}
}

func TestRoundingConfigForTickSizeMatchesOfficialV2(t *testing.T) {
	tests := []struct {
		tick           types.TickSize
		priceDecimals  int
		amountDecimals int
		tickUnits      int64
	}{
		{types.TickSize0_1, 1, 3, 1},
		{types.TickSize0_01, 2, 4, 1},
		{types.TickSize0_005, 3, 5, 5},
		{types.TickSize0_0025, 4, 6, 25},
		{types.TickSize0_001, 3, 5, 1},
		{types.TickSize0_0001, 4, 6, 1},
	}
	for _, tt := range tests {
		t.Run(string(tt.tick), func(t *testing.T) {
			config, ok := roundingConfigForTickSize(tt.tick)
			if !ok {
				t.Fatalf("missing rounding config for %s", tt.tick)
			}
			if config.priceDecimals != tt.priceDecimals ||
				config.amountDecimals != tt.amountDecimals ||
				config.tickUnits != tt.tickUnits {
				t.Fatalf("config for %s = %+v", tt.tick, config)
			}
		})
	}
}

func TestPriceUnitsOnTickAcceptsEveryOfficialGrid(t *testing.T) {
	tests := []struct {
		tick  types.TickSize
		price float64
	}{
		{types.TickSize0_1, 0.3},
		{types.TickSize0_01, 0.37},
		{types.TickSize0_005, 0.375},
		{types.TickSize0_0025, 0.3775},
		{types.TickSize0_001, 0.377},
		{types.TickSize0_0001, 0.3771},
		// 接受正常业务计算产生的 float64 尾差，但不改变真实价格网格。
		{types.TickSize0_1, 0.1 + 0.2},
	}
	for _, tt := range tests {
		t.Run(string(tt.tick), func(t *testing.T) {
			if _, _, err := priceUnitsOnTick(tt.price, tt.tick); err != nil {
				t.Fatalf("valid grid price %g for %s rejected: %v", tt.price, tt.tick, err)
			}
		})
	}
}

func TestPriceUnitsOnTickRejectsOffGridWithoutRounding(t *testing.T) {
	tests := []struct {
		tick  types.TickSize
		price float64
	}{
		{types.TickSize0_1, 0.15},
		{types.TickSize0_01, 0.371},
		{types.TickSize0_005, 0.377},
		{types.TickSize0_0025, 0.378},
		{types.TickSize0_001, 0.3771},
		{types.TickSize0_0001, 0.37711},
	}
	for _, tt := range tests {
		t.Run(string(tt.tick), func(t *testing.T) {
			_, _, err := priceUnitsOnTick(tt.price, tt.tick)
			if err == nil || !strings.Contains(err.Error(), "not aligned") {
				t.Fatalf("off-grid price %g for %s error = %v", tt.price, tt.tick, err)
			}
		})
	}
}

func TestValidateOrderTickSizesRejectsBeforeSubmission(t *testing.T) {
	tests := []struct {
		name    string
		orders  []types.OrderArgs
		wantErr string
	}{
		{
			name:    "missing tick identifies order",
			orders:  []types.OrderArgs{{TokenID: "first", Price: 0.5, TickSize: types.TickSize0_01}, {TokenID: "second", Price: 0.5}},
			wantErr: "订单 2 token=second: tick_size is required",
		},
		{
			name:    "price below supplied tick",
			orders:  []types.OrderArgs{{TokenID: "token", Price: 0.001, TickSize: types.TickSize0_01}},
			wantErr: "价格无效",
		},
		{
			name:    "nan price",
			orders:  []types.OrderArgs{{TokenID: "token", Price: math.NaN(), TickSize: types.TickSize0_01}},
			wantErr: "价格无效",
		},
		{
			name:    "price off 0.005 grid",
			orders:  []types.OrderArgs{{TokenID: "token", Price: 0.007, TickSize: types.TickSize0_005}},
			wantErr: "not aligned",
		},
		{
			name:    "price off 0.0025 grid",
			orders:  []types.OrderArgs{{TokenID: "token", Price: 0.007, TickSize: types.TickSize0_0025}},
			wantErr: "not aligned",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrderTickSizes(tt.orders)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateOrderTickSizes() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestCreateAndPostOrdersRejectsMissingTickBeforeSubmission(t *testing.T) {
	client := &orderClientImpl{}
	_, err := client.CreateAndPostOrders(
		[]types.OrderArgs{{TokenID: "token", Price: 0.5, Size: 5, Side: types.OrderSideBUY}},
		[]types.OrderType{types.OrderTypeGTC},
	)
	if err == nil || !strings.Contains(err.Error(), "tick_size is required") {
		t.Fatalf("CreateAndPostOrders() error = %v, want missing tick_size error", err)
	}
}

func TestCreateAndPostOrdersRejectsOffGridPriceBeforeSubmission(t *testing.T) {
	client := &orderClientImpl{}
	_, err := client.CreateAndPostOrders(
		[]types.OrderArgs{{TokenID: "token", Price: 0.007, Size: 5, Side: types.OrderSideBUY, TickSize: types.TickSize0_005}},
		[]types.OrderType{types.OrderTypeGTC},
	)
	if err == nil || !strings.Contains(err.Error(), "not aligned") {
		t.Fatalf("CreateAndPostOrders() error = %v, want off-grid error", err)
	}
}

func TestCalculateOrderAmountsMatchesOfficialV2RoundingConfig(t *testing.T) {
	tests := []struct {
		tick        types.TickSize
		price       float64
		quoteAmount string
	}{
		{types.TickSize0_1, 0.3, "3702000"},
		{types.TickSize0_01, 0.37, "4565800"},
		{types.TickSize0_005, 0.375, "4627500"},
		{types.TickSize0_0025, 0.3775, "4658350"},
		{types.TickSize0_001, 0.377, "4652180"},
		{types.TickSize0_0001, 0.3771, "4653414"},
	}
	client := &orderClientImpl{}
	for _, tt := range tests {
		t.Run(string(tt.tick), func(t *testing.T) {
			buyMaker, buyTaker, err := client.calculateOrderAmounts(types.OrderSideBUY, 12.349, tt.price, tt.tick)
			if err != nil {
				t.Fatalf("BUY calculateOrderAmounts: %v", err)
			}
			if buyMaker.String() != tt.quoteAmount || buyTaker.String() != "12340000" {
				t.Fatalf("BUY amounts = maker %s taker %s; want %s/12340000", buyMaker, buyTaker, tt.quoteAmount)
			}

			sellMaker, sellTaker, err := client.calculateOrderAmounts(types.OrderSideSELL, 12.349, tt.price, tt.tick)
			if err != nil {
				t.Fatalf("SELL calculateOrderAmounts: %v", err)
			}
			if sellMaker.String() != "12340000" || sellTaker.String() != tt.quoteAmount {
				t.Fatalf("SELL amounts = maker %s taker %s; want 12340000/%s", sellMaker, sellTaker, tt.quoteAmount)
			}
		})
	}
}
