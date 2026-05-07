package clob

import (
	"fmt"
	"strings"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

// fakeBadOrderbookErr 制造一个能被 extractBadOrderbookTokensFromErr 识别的
// "the orderbook X does not exist" 错误，模拟 CLOB top-level 400。
func fakeBadOrderbookErr(tokenID string) error {
	return fmt.Errorf("HTTP 400: the orderbook %s does not exist", tokenID)
}

// scriptedPostFn 按预设脚本依次返回结果，用于驱动 runV2BatchWithStrip 的剥离
// 重试。每一步都接收当前剩余 args，可以基于此动态生成成功响应。
type scriptedStep struct {
	// onCall 输入当前 args，输出本轮模拟的 (resp, err)。返回 err 触发剥离重试。
	onCall func(args []types.OrderArgs) ([]types.OrderPostResponse, error)
}

func newScriptedPostFn(t *testing.T, steps []scriptedStep) func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
	t.Helper()
	idx := 0
	return func(a []types.OrderArgs, _ []types.OrderType) ([]types.OrderPostResponse, error) {
		if idx >= len(steps) {
			t.Fatalf("post fn called %d times, exceeded scripted %d", idx+1, len(steps))
		}
		step := steps[idx]
		idx++
		return step.onCall(a)
	}
}

// TestStrip_SingleTokenAllStripped 单一 TokenID 全剥：3 笔订单同 TokenID，
// 第一轮 CLOB 报 "orderbook X does not exist" → 期望返回 [3]OrderPostResponse
// 全部 Status="stripped"，ErrorMsg 含该 token。
func TestStrip_SingleTokenAllStripped(t *testing.T) {
	const tokenA = "111"
	args := []types.OrderArgs{
		{TokenID: tokenA, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenA, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenA, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
	}
	otypes := []types.OrderType{types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC}
	postFn := newScriptedPostFn(t, []scriptedStep{
		{
			// 第一轮：CLOB 报 tokenA 不存在
			onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) {
				if len(a) != 3 {
					t.Fatalf("attempt 1: expected 3 args, got %d", len(a))
				}
				return nil, fakeBadOrderbookErr(tokenA)
			},
		},
		// 第二轮 args 为空时 SDK 不再调 postFn（直接返回 buildFinal(nil)）。
	})

	out, err := runV2BatchWithStrip(args, otypes, postFn)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len(out) = %d, want 3", len(out))
	}
	for i, r := range out {
		if r.Status != OrderStrippedStatus {
			t.Errorf("[%d] Status = %q, want %q", i, r.Status, OrderStrippedStatus)
		}
		if !strings.Contains(r.ErrorMsg, tokenA) {
			t.Errorf("[%d] ErrorMsg = %q, want token %s in it", i, r.ErrorMsg, tokenA)
		}
		if !strings.Contains(r.ErrorMsg, "stripped by SDK retry") {
			t.Errorf("[%d] ErrorMsg = %q missing stripped-by-SDK marker", i, r.ErrorMsg)
		}
	}
}

// TestStrip_PartialMultiToken 多 TokenID 部分剥：5 笔 = 2 笔 A（dead）+ 3 笔 B（good）。
// 第一轮报 A 死，第二轮成功。期望返回 [5]OrderPostResponse，A 位置 stripped，
// B 位置正常 OrderID。
func TestStrip_PartialMultiToken(t *testing.T) {
	const (
		tokenA = "111"
		tokenB = "222"
	)
	// input 顺序：[A, A, B, B, B]
	args := []types.OrderArgs{
		{TokenID: tokenA, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenA, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenB, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenB, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenB, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
	}
	otypes := []types.OrderType{
		types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC,
	}
	postFn := newScriptedPostFn(t, []scriptedStep{
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) {
			if len(a) != 5 {
				t.Fatalf("attempt 1: expected 5 args, got %d", len(a))
			}
			return nil, fakeBadOrderbookErr(tokenA)
		}},
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) {
			// 剥离后剩 3 笔 B
			if len(a) != 3 {
				t.Fatalf("attempt 2: expected 3 args after strip, got %d", len(a))
			}
			for _, x := range a {
				if x.TokenID != tokenB {
					t.Fatalf("attempt 2: expected only tokenB, got %s", x.TokenID)
				}
			}
			// 把 OrderID 设成 "ok-<token>-<i>" 以便外层验证位置正确
			out := make([]types.OrderPostResponse, len(a))
			for i := range a {
				out[i] = types.OrderPostResponse{
					OrderID: types.Keccak256(fmt.Sprintf("ok-%s-%d", a[i].TokenID, i)),
					Status:  "matched",
				}
			}
			return out, nil
		}},
	})

	out, err := runV2BatchWithStrip(args, otypes, postFn)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("len(out) = %d, want 5", len(out))
	}
	// 下标 0,1（A）应为 stripped
	for _, i := range []int{0, 1} {
		if out[i].Status != OrderStrippedStatus {
			t.Errorf("[%d] Status = %q, want %q", i, out[i].Status, OrderStrippedStatus)
		}
		if !strings.Contains(out[i].ErrorMsg, tokenA) {
			t.Errorf("[%d] ErrorMsg = %q, want tokenA in it", i, out[i].ErrorMsg)
		}
	}
	// 下标 2,3,4（B）应有 OrderID 且无错
	for _, i := range []int{2, 3, 4} {
		if out[i].OrderID == "" {
			t.Errorf("[%d] OrderID empty", i)
		}
		if out[i].Status == OrderStrippedStatus {
			t.Errorf("[%d] should not be stripped", i)
		}
		if out[i].ErrorMsg != "" {
			t.Errorf("[%d] ErrorMsg = %q, want empty", i, out[i].ErrorMsg)
		}
	}
}

// TestStrip_PreservesOriginalOrder 交错的 input [A, B, A, B, A]：A 死 B 活。
// 验证下标 0/2/4（A）是 stripped，下标 1/3（B）有正常响应，且 1 号位响应对应
// 原 input[1] 的 B（用 OrderID 携带原始 token 来核验）。
func TestStrip_PreservesOriginalOrder(t *testing.T) {
	const (
		tokenA = "777"
		tokenB = "888"
	)
	args := []types.OrderArgs{
		{TokenID: tokenA, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenB, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenA, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenB, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: tokenA, Price: 0.5, Size: 5, Side: types.OrderSideBUY},
	}
	otypes := []types.OrderType{
		types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC,
	}
	postFn := newScriptedPostFn(t, []scriptedStep{
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) {
			return nil, fakeBadOrderbookErr(tokenA)
		}},
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) {
			// 剥后剩两笔，且都是 tokenB；按相对顺序对应原 input[1], input[3]
			if len(a) != 2 {
				t.Fatalf("attempt 2: expected 2 args after strip, got %d", len(a))
			}
			out := make([]types.OrderPostResponse, len(a))
			for i := range a {
				// OrderID 携带 tokenID 让外层断言对位
				out[i] = types.OrderPostResponse{
					OrderID: types.Keccak256(fmt.Sprintf("OK-%s", a[i].TokenID)),
				}
			}
			return out, nil
		}},
	})

	out, err := runV2BatchWithStrip(args, otypes, postFn)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("len(out) = %d, want 5", len(out))
	}
	// A: 0,2,4 stripped
	for _, i := range []int{0, 2, 4} {
		if out[i].Status != OrderStrippedStatus {
			t.Errorf("[%d] Status = %q, want %q", i, out[i].Status, OrderStrippedStatus)
		}
	}
	// B: 1,3 应该是 OK-LIVE
	for _, i := range []int{1, 3} {
		want := "OK-" + tokenB
		if string(out[i].OrderID) != want {
			t.Errorf("[%d] OrderID = %q, want %q (顺序错位)", i, out[i].OrderID, want)
		}
	}
}

// TestStrip_RetryExhausted 退化场景：retry 上限耗尽 → 返回 nil + error，
// 不做长度补齐（与现有失败路径行为一致）。
func TestStrip_RetryExhausted(t *testing.T) {
	args := []types.OrderArgs{
		{TokenID: "101", Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: "102", Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: "103", Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: "104", Price: 0.5, Size: 5, Side: types.OrderSideBUY},
	}
	otypes := []types.OrderType{
		types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC,
	}
	// 每轮报一个不同的坏 token，用尽 (max+1) = 4 次都不放过
	steps := []scriptedStep{
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) { return nil, fakeBadOrderbookErr("101") }},
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) { return nil, fakeBadOrderbookErr("102") }},
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) { return nil, fakeBadOrderbookErr("103") }},
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) { return nil, fakeBadOrderbookErr("104") }},
	}
	postFn := newScriptedPostFn(t, steps)

	out, err := runV2BatchWithStrip(args, otypes, postFn)
	if err == nil {
		t.Fatalf("expected error on retry exhaustion, got nil")
	}
	if out != nil {
		t.Fatalf("expected nil result on failure, got len=%d", len(out))
	}
}

// TestStrip_NonBadTokenError 非 "orderbook X does not exist" 错误：直接透传，
// 不剥离不重试，也不做长度补齐。
func TestStrip_NonBadTokenError(t *testing.T) {
	args := []types.OrderArgs{{TokenID: "999", Price: 0.5, Size: 5, Side: types.OrderSideBUY}}
	otypes := []types.OrderType{types.OrderTypeGTC}
	postFn := newScriptedPostFn(t, []scriptedStep{
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) {
			return nil, fmt.Errorf("HTTP 500: internal server error")
		}},
	})
	out, err := runV2BatchWithStrip(args, otypes, postFn)
	if err == nil || !strings.Contains(err.Error(), "internal server error") {
		t.Fatalf("expected passthrough error, got %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil result, got %v", out)
	}
}

// TestStrip_HappyPathPassthrough 一次性成功（无剥离）：返回数组与 postFn 输出
// 等长且按原始顺序，未触发任何回填。
func TestStrip_HappyPathPassthrough(t *testing.T) {
	args := []types.OrderArgs{
		{TokenID: "X", Price: 0.5, Size: 5, Side: types.OrderSideBUY},
		{TokenID: "Y", Price: 0.5, Size: 5, Side: types.OrderSideBUY},
	}
	otypes := []types.OrderType{types.OrderTypeGTC, types.OrderTypeGTC}
	postFn := newScriptedPostFn(t, []scriptedStep{
		{onCall: func(a []types.OrderArgs) ([]types.OrderPostResponse, error) {
			return []types.OrderPostResponse{
				{OrderID: "ord-X"},
				{OrderID: "ord-Y"},
			}, nil
		}},
	})
	out, err := runV2BatchWithStrip(args, otypes, postFn)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 || out[0].OrderID != "ord-X" || out[1].OrderID != "ord-Y" {
		t.Fatalf("happy-path result mismatch: %+v", out)
	}
}
