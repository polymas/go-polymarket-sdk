package clob

import (
	"strings"
	"testing"
	"time"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestValidateOrderLifetimes(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	tests := []struct {
		name      string
		orderType types.OrderType
		expires   int64
		postOnly  bool
		wantError string
	}{
		{name: "GTC", orderType: types.OrderTypeGTC},
		{name: "GTC post only", orderType: types.OrderTypeGTC, postOnly: true},
		{name: "GTD minimum", orderType: types.OrderTypeGTD, expires: now.Unix() + 180},
		{name: "GTD post only", orderType: types.OrderTypeGTD, expires: now.Unix() + 181, postOnly: true},
		{name: "FOK", orderType: types.OrderTypeFOK},
		{name: "FAK", orderType: types.OrderTypeFAK},
		{name: "GTD missing expiration", orderType: types.OrderTypeGTD, wantError: "3 分钟"},
		{name: "GTD too soon", orderType: types.OrderTypeGTD, expires: now.Unix() + 179, wantError: "3 分钟"},
		{name: "GTC expiration", orderType: types.OrderTypeGTC, expires: 1, wantError: "不允许设置"},
		{name: "FOK expiration", orderType: types.OrderTypeFOK, expires: 1, wantError: "不允许设置"},
		{name: "FAK post only", orderType: types.OrderTypeFAK, postOnly: true, wantError: "仅支持 GTC 或 GTD"},
		{name: "FOK post only", orderType: types.OrderTypeFOK, postOnly: true, wantError: "仅支持 GTC 或 GTD"},
		{name: "unknown", orderType: types.OrderType("IOC"), wantError: "不支持的订单类型"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOrderLifetimes(
				[]types.OrderArgs{{Expiration: tt.expires, PostOnly: tt.postOnly}},
				[]types.OrderType{tt.orderType},
				now,
			)
			if tt.wantError == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantError)
			}
		})
	}
}
