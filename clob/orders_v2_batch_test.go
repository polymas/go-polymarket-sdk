package clob

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdkerrors "github.com/polymas/go-polymarket-sdk/errors"
	"github.com/polymas/go-polymarket-sdk/types"
)

func fakeBadOrderbookErr(tokenID string) error {
	return fmt.Errorf("HTTP 400: the orderbook %s does not exist", tokenID)
}

func TestResolveV2BatchClosedMarketIsTerminalWithoutRetry(t *testing.T) {
	const tokenID = "111"
	args := []types.OrderArgs{{TokenID: tokenID}, {TokenID: tokenID}}
	typesList := []types.OrderType{types.OrderTypeGTC, types.OrderTypeGTC}
	calls := 0

	results, err := resolveV2BatchAttempt(args, typesList, func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
		calls++
		return nil, fakeBadOrderbookErr(tokenID)
	})
	if err != nil {
		t.Fatalf("closed-only batch should be a handled terminal result: %v", err)
	}
	if calls != 1 {
		t.Fatalf("post called %d times, want exactly once", calls)
	}
	if len(results) != len(args) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(args))
	}
	for i, result := range results {
		if result.Status != OrderMarketClosedStatus || !strings.Contains(result.ErrorMsg, tokenID) {
			t.Fatalf("result[%d] = %+v, want market_closed", i, result)
		}
	}
}

func TestResolveV2BatchMixedClosedMarketDoesNotRetrySiblings(t *testing.T) {
	const (
		closedToken = "111"
		otherToken  = "222"
	)
	args := []types.OrderArgs{{TokenID: closedToken}, {TokenID: otherToken}, {TokenID: closedToken}}
	typesList := []types.OrderType{types.OrderTypeGTC, types.OrderTypeGTC, types.OrderTypeGTC}
	calls := 0

	results, err := resolveV2BatchAttempt(args, typesList, func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
		calls++
		return nil, fakeBadOrderbookErr(closedToken)
	})
	if err == nil || !strings.Contains(err.Error(), "other orders were not submitted") {
		t.Fatalf("mixed closed batch error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("post called %d times, want no retry", calls)
	}
	if results[0].Status != OrderMarketClosedStatus || results[2].Status != OrderMarketClosedStatus {
		t.Fatalf("closed token results = %+v", results)
	}
	if results[1].Status != OrderNotSubmittedStatus {
		t.Fatalf("sibling result = %+v, want not_submitted", results[1])
	}
}

func TestResolveV2BatchClassifiesRequestLevelErrors(t *testing.T) {
	args := []types.OrderArgs{{TokenID: "111"}, {TokenID: "222"}}
	typesList := []types.OrderType{types.OrderTypeGTC, types.OrderTypeGTC}

	t.Run("top-level 4xx is not submitted", func(t *testing.T) {
		results, err := resolveV2BatchAttempt(args, typesList, func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
			return nil, fmt.Errorf("HTTP 401: Unauthorized/Invalid api key")
		})
		if err == nil {
			t.Fatal("expected error")
		}
		for _, result := range results {
			if result.Status != OrderNotSubmittedStatus {
				t.Fatalf("result = %+v, want not_submitted", result)
			}
		}
	})

	t.Run("transport error is unknown", func(t *testing.T) {
		results, err := resolveV2BatchAttempt(args, typesList, func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
			return nil, fmt.Errorf("request failed: timeout")
		})
		if err == nil {
			t.Fatal("expected error")
		}
		for _, result := range results {
			if result.Status != OrderUnknownStatus {
				t.Fatalf("result = %+v, want unknown", result)
			}
		}
	})

	t.Run("local build error is not submitted", func(t *testing.T) {
		results, err := resolveV2BatchAttempt(args, typesList, func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
			return nil, newBatchNotSubmittedError("local signing failed")
		})
		if err == nil {
			t.Fatal("expected error")
		}
		for _, result := range results {
			if result.Status != OrderNotSubmittedStatus {
				t.Fatalf("result = %+v, want not_submitted", result)
			}
		}
	})
}

func TestResolveV2BatchPreservesExpectedOrderIDOnAmbiguousFailure(t *testing.T) {
	args := []types.OrderArgs{{TokenID: "111"}, {TokenID: "222"}}
	typesList := []types.OrderType{types.OrderTypeGTC, types.OrderTypeGTC}
	expected := []types.Keccak256{
		"0x1111111111111111111111111111111111111111111111111111111111111111",
		"0x2222222222222222222222222222222222222222222222222222222222222222",
	}
	results, err := resolveV2BatchAttempt(args, typesList, func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
		return nil, &batchPostError{
			err:              fmt.Errorf("request failed: timeout"),
			expectedOrderIDs: expected,
			timing:           types.OrderResponseTiming{PostDuration: time.Second},
		}
	})
	if err == nil {
		t.Fatal("expected ambiguous submission error")
	}
	for i := range results {
		if results[i].Status != OrderUnknownStatus || results[i].ExpectedOrderID != expected[i] || results[i].Timing.PostDuration != time.Second {
			t.Fatalf("result[%d] = %+v", i, results[i])
		}
	}
}

func TestNormalizeV2BatchResponsesRequiresExactAlignment(t *testing.T) {
	results, err := normalizeV2BatchResponses([]types.OrderPostResponse{
		{OrderID: "order-1", Status: "live"},
		{ErrorMsg: "not enough balance / allowance"},
	}, 3)
	if err == nil || !strings.Contains(err.Error(), "got 2, want 3") {
		t.Fatalf("count mismatch error = %v", err)
	}
	if len(results) != 3 || results[0].OrderID != "order-1" {
		t.Fatalf("aligned results = %+v", results)
	}
	if results[1].Status != OrderServerRejectedStatus {
		t.Fatalf("per-order rejection = %+v", results[1])
	}
	if results[2].Status != OrderUnknownStatus {
		t.Fatalf("missing response = %+v", results[2])
	}
}

func TestNormalizeV2BatchResponsesKeepsPerOrderRejection(t *testing.T) {
	results, err := normalizeV2BatchResponses([]types.OrderPostResponse{
		{OrderID: "order-1", Status: "live"},
		{ErrorMsg: "not enough balance / allowance"},
	}, 2)
	if err != nil {
		t.Fatalf("mixed per-order response returned batch error: %v", err)
	}
	if results[0].Status != "live" || results[1].Status != OrderServerRejectedStatus {
		t.Fatalf("results = %+v", results)
	}
}

func TestRunOrderBatchesRunsBatchesConcurrentlyAndPreservesIndexes(t *testing.T) {
	args := make([]types.OrderArgs, 32)
	typesList := make([]types.OrderType, 32)
	for i := range args {
		args[i].TokenID = fmt.Sprintf("%d", i+1)
		typesList[i] = types.OrderTypeGTC
	}
	var calls atomic.Int32
	results, err := runOrderBatches(args, typesList, 15, func(batch []types.OrderArgs, _ []types.OrderType, _ ...bool) ([]types.OrderPostResponse, error) {
		calls.Add(1)
		// 第二个子批（token 16-30）模拟超时；其它子批正常。子批并发，不能靠调用次序判断。
		if batch[0].TokenID == "16" {
			return makeBatchStatusResults(len(batch), OrderUnknownStatus, "timeout"), fmt.Errorf("request failed: timeout")
		}
		out := make([]types.OrderPostResponse, len(batch))
		for i := range out {
			out[i] = types.OrderPostResponse{OrderID: types.Keccak256("ok-" + batch[i].TokenID), Status: "live"}
		}
		return out, nil
	})
	if err == nil || calls.Load() != 3 {
		t.Fatalf("err=%v calls=%d, want one failing batch out of three concurrent calls", err, calls.Load())
	}
	if !strings.Contains(err.Error(), "批次 2/3") {
		t.Fatalf("err = %v, want it to name batch 2/3", err)
	}
	if len(results) != len(args) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(args))
	}
	for i := 0; i < 15; i++ {
		if results[i].Status != "live" || results[i].OrderID != types.Keccak256(fmt.Sprintf("ok-%d", i+1)) {
			t.Fatalf("result[%d] = %+v, want live ok-%d", i, results[i], i+1)
		}
	}
	for i := 15; i < 30; i++ {
		if results[i].Status != OrderUnknownStatus {
			t.Fatalf("result[%d] = %+v, want unknown", i, results[i])
		}
	}
	// 失败的子批不再阻止后面的子批：第三批照常提交成功。
	for i := 30; i < 32; i++ {
		if results[i].Status != "live" || results[i].OrderID != types.Keccak256(fmt.Sprintf("ok-%d", i+1)) {
			t.Fatalf("result[%d] = %+v, want live ok-%d", i, results[i], i+1)
		}
	}
}

func TestRunOrderBatchesMarksClosedTokenInEveryBatch(t *testing.T) {
	const tokenID = "111"
	args := make([]types.OrderArgs, 20)
	typesList := make([]types.OrderType, 20)
	for i := range args {
		args[i].TokenID = tokenID
		typesList[i] = types.OrderTypeGTC
	}
	var calls atomic.Int32
	results, err := runOrderBatches(args, typesList, 15, func(batch []types.OrderArgs, _ []types.OrderType, _ ...bool) ([]types.OrderPostResponse, error) {
		calls.Add(1)
		return resolveV2BatchAttempt(batch, typesList[:len(batch)], func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
			return nil, fakeBadOrderbookErr(tokenID)
		})
	})
	if err != nil {
		t.Fatalf("runOrderBatches: %v", err)
	}
	// 子批并发提交，无法用前一批的结果拦截后一批；两批各自把已关闭市场标为 market_closed。
	if calls.Load() != 2 {
		t.Fatalf("post called %d times, want both concurrent batches submitted", calls.Load())
	}
	for i, result := range results {
		if result.Status != OrderMarketClosedStatus {
			t.Fatalf("result[%d] = %+v, want market_closed", i, result)
		}
	}
}

func TestExtractBadOrderbookTokensFromErr(t *testing.T) {
	err := fmt.Errorf("HTTP 400: the orderbook 111 does not exist; the orderbook 222 does not exist; the orderbook 111 does not exist")
	tokens := extractBadOrderbookTokensFromErr(err)
	if len(tokens) != 2 || tokens[0] != "111" || tokens[1] != "222" {
		t.Fatalf("tokens = %v", tokens)
	}
}

func TestIsTopLevelHTTP4xx(t *testing.T) {
	apiErr := &sdkerrors.APIError{
		Service: "clob", Method: "POST", Path: "/orders",
		Status: 400, Message: "invalid price 0.999 for tick size 0.01",
	}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		// 回归：APIError.Error() 的状态码前面还有 "POST clob /orders: "，
		// 老的 ^HTTP 4\d\d: 锚定正则在这里必然失配，把明确拒绝错记成 unknown。
		{"typed 4xx", apiErr, true},
		{"typed 4xx wrapped", fmt.Errorf("post orders: %w", apiErr), true},
		{"typed 5xx", &sdkerrors.APIError{Status: 503, Path: "/orders"}, false},
		{"ambiguous wrapping 4xx", &sdkerrors.AmbiguousOutcomeError{
			Operation: "post orders", Cause: apiErr,
		}, false},
		{"untyped string fallback", fmt.Errorf("POST clob /orders: HTTP 400: bad tick"), true},
		{"plain transport error", fmt.Errorf("dial tcp: i/o timeout"), false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTopLevelHTTP4xx(tc.err); got != tc.want {
				t.Fatalf("isTopLevelHTTP4xx(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestResolveV2BatchAttemptTypedHTTP400 锁定端到端行为：CLOB 明确 400 拒绝时
// 每单都是 not_submitted，而不是 unknown（后者会让上层发"可能已成交"告警）。
func TestResolveV2BatchAttemptTypedHTTP400(t *testing.T) {
	apiErr := &sdkerrors.APIError{
		Service: "clob", Method: "POST", Path: "/orders",
		Status: 400, Message: "invalid price",
	}
	args := []types.OrderArgs{{TokenID: "111"}, {TokenID: "222"}}
	typesList := []types.OrderType{types.OrderTypeGTC, types.OrderTypeGTC}
	results, err := resolveV2BatchAttempt(args, typesList, func([]types.OrderArgs, []types.OrderType) ([]types.OrderPostResponse, error) {
		return nil, apiErr
	})
	if err == nil {
		t.Fatal("want error surfaced to caller")
	}
	for i, result := range results {
		if result.Status != OrderNotSubmittedStatus {
			t.Fatalf("result[%d] = %+v, want not_submitted", i, result)
		}
	}
}
