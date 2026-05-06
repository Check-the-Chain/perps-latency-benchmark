package cost

import (
	"encoding/json"
	"fmt"

	"perps-latency-benchmark/internal/bench"
)

func decodeBalance(value any) (bench.BalanceSnapshot, error) {
	var balance bench.BalanceSnapshot
	if value == nil {
		return balance, fmt.Errorf("cost command did not return balance metadata")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return balance, err
	}
	if err := json.Unmarshal(data, &balance); err != nil {
		return balance, err
	}
	return balance, nil
}

func decodeCost(value any) (bench.SampleCost, error) {
	var cost bench.SampleCost
	if value == nil {
		return cost, fmt.Errorf("cost command did not return cost metadata")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return cost, err
	}
	if err := json.Unmarshal(data, &cost); err != nil {
		return cost, err
	}
	return cost, nil
}
