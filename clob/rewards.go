package clob

import (
	"fmt"

	"github.com/polymas/go-polymarket-sdk/http"
	"github.com/polymas/go-polymarket-sdk/internal"
	"github.com/polymas/go-polymarket-sdk/types"
)

// IsOrderScoring 检查订单是否计分
func (c *rewardClientImpl) IsOrderScoring(orderID types.Keccak256) (bool, error) {
	params := map[string]string{"order_id": string(orderID)}

	var result struct {
		Scoring bool `json:"scoring"`
	}

	resp, err := http.Get[struct {
		Scoring bool `json:"scoring"`
	}](c.baseClient.baseURL, internal.IsOrderScoring, params)
	if err != nil {
		return false, fmt.Errorf("failed to check order scoring: %w", err)
	}
	result = *resp

	return result.Scoring, nil
}

// AreOrdersScoring 批量检查订单是否计分
func (c *rewardClientImpl) AreOrdersScoring(orderIDs []types.Keccak256) (map[types.Keccak256]bool, error) {
	if len(orderIDs) == 0 {
		return make(map[types.Keccak256]bool), nil
	}

	// Build request body
	orderIDStrings := make([]string, len(orderIDs))
	for i, orderID := range orderIDs {
		orderIDStrings[i] = string(orderID)
	}

	requestBody := map[string][]string{
		"order_ids": orderIDStrings,
	}

	// Make POST request
	var result map[string]bool
	resp, err := http.Post[map[string]bool](c.baseClient.baseURL, internal.AreOrdersScoring, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to check orders scoring: %w", err)
	}
	result = *resp

	// Convert string keys to Keccak256
	resultMap := make(map[types.Keccak256]bool, len(result))
	for k, v := range result {
		resultMap[types.Keccak256(k)] = v
	}

	return resultMap, nil
}
