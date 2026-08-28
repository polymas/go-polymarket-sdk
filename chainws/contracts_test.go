package chainws

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
)

func TestPolygonContractRegistryDefaultsToV2(t *testing.T) {
	registry := PolygonContractRegistry(false)
	names := make(map[string]ContractSpec, len(registry))
	seen := make(map[common.Address]bool, len(registry))
	for _, contract := range registry {
		if contract.Address == (common.Address{}) {
			t.Fatalf("%s has zero address", contract.Name)
		}
		if seen[contract.Address] {
			t.Fatalf("duplicate address %s", contract.Address)
		}
		seen[contract.Address] = true
		names[contract.Name] = contract
		if contract.Version != ContractVersionV2 {
			t.Fatalf("default registry contains %s contract %s", contract.Version, contract.Name)
		}
	}
	for _, name := range []string{"pUSD", "USDC.e", "ConditionalTokens", "ExchangeV2", "NegRiskExchangeV2", "CollateralOnramp", "CollateralOfframp", "CtfCollateralAdapter", "NegRiskCtfCollateralAdapter", "AutoRedeemOperator"} {
		if _, ok := names[name]; !ok {
			t.Errorf("default registry is missing %s", name)
		}
	}
	for _, address := range []string{internal.PolygonExchange, internal.PolygonNegRiskExchange, internal.PolygonNegRiskAdapter} {
		if seen[common.HexToAddress(address)] {
			t.Errorf("default registry unexpectedly contains legacy %s", address)
		}
	}
}

func TestPolygonContractRegistryLegacyOptIn(t *testing.T) {
	registry := PolygonContractRegistry(true)
	want := map[common.Address]bool{
		common.HexToAddress(internal.PolygonExchange):        false,
		common.HexToAddress(internal.PolygonNegRiskExchange): false,
		common.HexToAddress(internal.PolygonNegRiskAdapter):  false,
	}
	for _, contract := range registry {
		if _, ok := want[contract.Address]; ok {
			want[contract.Address] = contract.Version == ContractVersionLegacy
		}
	}
	for address, valid := range want {
		if !valid {
			t.Errorf("legacy registry is missing or mislabels %s", address)
		}
	}
}

func TestCurrentClientSubscribesToTransferBatch(t *testing.T) {
	client, err := NewClient(internal.Polygon, "wss://example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	impl := client.(*chainWSClient)
	queries := impl.buildTopicFilterQueries([]common.Address{common.HexToAddress("0x1")})
	if len(queries) != 8 {
		t.Fatalf("got %d filter queries, want 8", len(queries))
	}
	if queries[6].Topics[0][0] != transferBatchEventSig || queries[7].Topics[0][0] != transferBatchEventSig {
		t.Fatal("TransferBatch from/to filters are missing")
	}
}
