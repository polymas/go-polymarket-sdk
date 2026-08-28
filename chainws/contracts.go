package chainws

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/polymas/go-polymarket-sdk/internal"
)

// ContractVersion identifies whether a registry entry belongs to the current
// V2 stack or is retained only for opt-in historical monitoring.
type ContractVersion string

const (
	ContractVersionV2     ContractVersion = "v2"
	ContractVersionLegacy ContractVersion = "legacy"
)

// ContractPurpose describes why chainws subscribes to a contract.
type ContractPurpose string

const (
	ContractPurposeCollateral ContractPurpose = "collateral"
	ContractPurposePosition   ContractPurpose = "position"
	ContractPurposeExchange   ContractPurpose = "exchange"
	ContractPurposeAdapter    ContractPurpose = "adapter"
)

// ContractSpec is one versioned Polygon contract monitored by chainws.
type ContractSpec struct {
	Name    string
	Address common.Address
	Version ContractVersion
	Purpose ContractPurpose
}

// PolygonContractRegistry returns the current V2 monitoring set. Legacy V1
// exchanges and the retired adapter are included only when includeLegacy is
// true. Addresses follow the current official ts-sdk/py-sdk production config.
func PolygonContractRegistry(includeLegacy bool) []ContractSpec {
	contracts := []ContractSpec{
		{Name: "pUSD", Address: common.HexToAddress(internal.PolygonPUSD), Version: ContractVersionV2, Purpose: ContractPurposeCollateral},
		{Name: "USDC.e", Address: common.HexToAddress(internal.PolygonCollateral), Version: ContractVersionV2, Purpose: ContractPurposeCollateral},
		{Name: "ConditionalTokens", Address: common.HexToAddress(internal.PolygonConditionalTokens), Version: ContractVersionV2, Purpose: ContractPurposePosition},
		{Name: "ExchangeV2", Address: common.HexToAddress(internal.PolygonExchangeV2), Version: ContractVersionV2, Purpose: ContractPurposeExchange},
		{Name: "NegRiskExchangeV2", Address: common.HexToAddress(internal.PolygonNegRiskExchangeV2), Version: ContractVersionV2, Purpose: ContractPurposeExchange},
		{Name: "CollateralOnramp", Address: common.HexToAddress(internal.PolygonCollateralOnramp), Version: ContractVersionV2, Purpose: ContractPurposeAdapter},
		{Name: "CollateralOfframp", Address: common.HexToAddress(internal.PolygonCollateralOfframp), Version: ContractVersionV2, Purpose: ContractPurposeAdapter},
		{Name: "CtfCollateralAdapter", Address: common.HexToAddress(internal.PolygonCtfCollateralAdapter), Version: ContractVersionV2, Purpose: ContractPurposeAdapter},
		{Name: "NegRiskCtfCollateralAdapter", Address: common.HexToAddress(internal.PolygonNegRiskCtfCollateralAdapter), Version: ContractVersionV2, Purpose: ContractPurposeAdapter},
		{Name: "AutoRedeemOperator", Address: common.HexToAddress(internal.PolymarketAutoClaimer), Version: ContractVersionV2, Purpose: ContractPurposeAdapter},
	}
	if includeLegacy {
		contracts = append(contracts,
			ContractSpec{Name: "ExchangeV1", Address: common.HexToAddress(internal.PolygonExchange), Version: ContractVersionLegacy, Purpose: ContractPurposeExchange},
			ContractSpec{Name: "NegRiskExchangeV1", Address: common.HexToAddress(internal.PolygonNegRiskExchange), Version: ContractVersionLegacy, Purpose: ContractPurposeExchange},
			ContractSpec{Name: "NegRiskAdapterV1", Address: common.HexToAddress(internal.PolygonNegRiskAdapter), Version: ContractVersionLegacy, Purpose: ContractPurposeAdapter},
		)
	}
	return contracts
}

func contractAddresses(registry []ContractSpec) []common.Address {
	seen := make(map[common.Address]struct{}, len(registry))
	addresses := make([]common.Address, 0, len(registry))
	for _, contract := range registry {
		if contract.Address == (common.Address{}) {
			continue
		}
		if _, ok := seen[contract.Address]; ok {
			continue
		}
		seen[contract.Address] = struct{}{}
		addresses = append(addresses, contract.Address)
	}
	return addresses
}
