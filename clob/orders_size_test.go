package clob

import (
	"math"
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestValidateOrderSizesAcceptsDefaultMinimumAndAbove(t *testing.T) {
	orders := []types.OrderArgs{
		{TokenID: "minimum", Size: 5},
		{TokenID: "above", Size: 5.01},
	}
	if err := validateOrderSizes(orders); err != nil {
		t.Fatalf("valid sizes rejected: %v", err)
	}
}

func TestValidateOrderSizesRejectsInvalidBatchItem(t *testing.T) {
	tests := []struct {
		name string
		size float64
	}{
		{name: "below minimum", size: 4.99},
		{name: "zero", size: 0},
		{name: "negative", size: -1},
		{name: "nan", size: math.NaN()},
		{name: "positive infinity", size: math.Inf(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrderSizes([]types.OrderArgs{
				{TokenID: "valid", Size: 5},
				{TokenID: "invalid", Size: tt.size},
			})
			if err == nil || !strings.Contains(err.Error(), "订单 2 token=invalid") ||
				!strings.Contains(err.Error(), "默认最小值 5") {
				t.Fatalf("validateOrderSizes() error = %v", err)
			}
		})
	}
}

func TestCreateAndPostOrdersRejectsUndersizedOrderBeforeSubmission(t *testing.T) {
	client := &orderClientImpl{}
	_, err := client.CreateAndPostOrders(
		[]types.OrderArgs{
			{TokenID: "valid", Price: 0.5, Size: 5, Side: types.OrderSideBUY, TickSize: types.TickSize0_01},
			{TokenID: "undersized", Price: 0.5, Size: 1, Side: types.OrderSideBUY, TickSize: types.TickSize0_01},
		},
		[]types.OrderType{types.OrderTypeGTC, types.OrderTypeGTC},
	)
	if err == nil || !strings.Contains(err.Error(), "订单 2 token=undersized") {
		t.Fatalf("CreateAndPostOrders() error = %v, want indexed minimum-size error", err)
	}
}
