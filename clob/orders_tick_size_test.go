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

func TestCalculateOrderAmountsUsesCallerSuppliedTickSize(t *testing.T) {
	client := &orderClientImpl{}

	coarseMaker, coarseTaker, err := client.calculateOrderAmounts(types.OrderSideBUY, 5, 0.1234, types.TickSize0_01)
	if err != nil {
		t.Fatalf("calculateOrderAmounts(coarse tick): %v", err)
	}
	fineMaker, fineTaker, err := client.calculateOrderAmounts(types.OrderSideBUY, 5, 0.1234, types.TickSize0_001)
	if err != nil {
		t.Fatalf("calculateOrderAmounts(fine tick): %v", err)
	}

	if coarseMaker.String() != "600000" || fineMaker.String() != "615000" {
		t.Fatalf("maker amounts = coarse %s, fine %s; want 600000 and 615000", coarseMaker, fineMaker)
	}
	if coarseTaker.String() != "5000000" || fineTaker.String() != "5000000" {
		t.Fatalf("taker amounts = coarse %s, fine %s; want both 5000000", coarseTaker, fineTaker)
	}
}
