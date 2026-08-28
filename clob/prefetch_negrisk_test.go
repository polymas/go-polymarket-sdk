package clob

import (
	"reflect"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

// TestNegRiskFetchList 验证并发预取的"待查列表"计算：跳过调用方已传入
// NegRisk 的 token、跳过缓存已命中的 token、对重复 token 去重并保序。
// 这是预取省网络开销的核心判定逻辑，纯函数、无网络、进 CI。
func TestNegRiskFetchList(t *testing.T) {
	truePtr, falsePtr := true, false
	c := &baseClient{
		negRisk: newNegRiskCache(),
	}
	c.negRisk.set("400", true, negRiskSourceManual)

	args := []types.OrderArgs{
		{TokenID: "100", NegRisk: &truePtr},  // 已传入 → 跳过
		{TokenID: "200"},                     // 待查
		{TokenID: "300"},                     // 待查
		{TokenID: "200"},                     // 重复 → 去重
		{TokenID: "400"},                     // 缓存命中 → 跳过
		{TokenID: "500", NegRisk: &falsePtr}, // 已传入(false) → 跳过
		{TokenID: "300"},                     // 重复 → 去重
	}

	got := c.negRiskFetchList(args)
	want := []string{"200", "300"} // 仅未传入且未缓存者，首次出现顺序去重
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("negRiskFetchList = %v, want %v", got, want)
	}
}

// TestNegRiskFetchList_AllKnown 验证：当所有 token 的状态都已知（传入或缓存）
// 时，待查列表为空——预取应完全不发请求。
func TestNegRiskFetchList_AllKnown(t *testing.T) {
	truePtr := true
	c := &baseClient{negRisk: newNegRiskCache()}
	c.negRisk.set("1", false, negRiskSourceManual)

	args := []types.OrderArgs{
		{TokenID: "1"},                    // 缓存命中
		{TokenID: "2", NegRisk: &truePtr}, // 传入
	}
	if got := c.negRiskFetchList(args); len(got) != 0 {
		t.Fatalf("expected empty fetch list, got %v", got)
	}
}
