package relayer

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
)

func TestV2WritableTargetsExcludeLegacyNegRiskAdapter(t *testing.T) {
	legacy := common.HexToAddress(internal.PolygonNegRiskAdapter)
	for _, address := range append(pusdSpendersV2(), ctfOperatorsV2()...) {
		if address == legacy {
			t.Fatalf("retired NegRiskAdapter remains in a V2 writable target list: %s", address.Hex())
		}
	}
}
