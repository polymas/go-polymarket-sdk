package data

import (
	"reflect"
	"testing"

	"github.com/polymas/go-polymarket-sdk/types"
)

func TestSumValueResponsesUsesEveryEntry(t *testing.T) {
	values := []types.ValueResponse{{Value: 1.25}, {Value: 2.75}, {Value: -0.5}}
	if got := sumValueResponses(values); got != 3.5 {
		t.Fatalf("sumValueResponses() = %v, want 3.5", got)
	}
	if got := sumValueResponses(nil); got != 0 {
		t.Fatalf("sumValueResponses(nil) = %v, want 0", got)
	}
}

func TestActivityTypedFilters(t *testing.T) {
	opts := &GetActivityOptions{}
	WithActivityType([]types.ActivityType{
		types.ActivityTypeDeposit,
		types.ActivityTypeWithdrawal,
	})(opts)
	WithActivityExcludeDepositsWithdrawals(false)(opts)

	wantTypes := []types.ActivityType{types.ActivityTypeDeposit, types.ActivityTypeWithdrawal}
	if !reflect.DeepEqual(opts.ActivityType, wantTypes) {
		t.Fatalf("ActivityType = %#v, want %#v", opts.ActivityType, wantTypes)
	}
	if opts.ExcludeDepositsWithdrawals == nil || *opts.ExcludeDepositsWithdrawals {
		t.Fatalf("ExcludeDepositsWithdrawals = %v, want false", opts.ExcludeDepositsWithdrawals)
	}
}
