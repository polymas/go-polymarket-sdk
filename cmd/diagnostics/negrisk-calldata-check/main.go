// negrisk_calldata_check：把我手撸的 encodeNegRiskRedeem 输出 vs go-ethereum 标准
// abi.Pack 的输出做 byte-by-byte 对比。一致就排除 ABI 编码 bug，500 锅在 relayer 后端
// 或 body 形态。不一致就是我们的问题，直接修。
//
// 用法：
//
//	go run ./cmd/diagnostics/negrisk-calldata-check
package main

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// 复制 SDK web3/gasless_transaction_v2.go 里的 encodeNegRiskRedeem，独立可跑
func encodeNegRiskRedeemManual(conditionIDHex string, amounts []*big.Int) []byte {
	selector := crypto.Keccak256([]byte("redeemPositions(bytes32,uint256[])"))[:4]
	condHash := common.HexToHash(conditionIDHex)

	data := append([]byte{}, selector...)
	data = append(data, condHash.Bytes()...)

	offset := big.NewInt(0x40)
	offsetBytes := make([]byte, 32)
	offset.FillBytes(offsetBytes)
	data = append(data, offsetBytes...)

	arrayLen := big.NewInt(int64(len(amounts)))
	lenBytes := make([]byte, 32)
	arrayLen.FillBytes(lenBytes)
	data = append(data, lenBytes...)

	for _, v := range amounts {
		elemBytes := make([]byte, 32)
		v.FillBytes(elemBytes)
		data = append(data, elemBytes...)
	}

	return data
}

func encodeNegRiskRedeemABI(conditionIDHex string, amounts []*big.Int) ([]byte, error) {
	const abiJSON = `[{
		"inputs": [
			{"name":"conditionId","type":"bytes32"},
			{"name":"amounts","type":"uint256[]"}
		],
		"name": "redeemPositions",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`
	parsed, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, err
	}
	var condArr [32]byte
	copy(condArr[:], common.HexToHash(conditionIDHex).Bytes())
	return parsed.Pack("redeemPositions", condArr, amounts)
}

func main() {
	cases := []struct {
		name        string
		conditionID string
		amounts     []*big.Int
	}{
		{"binary YES only", "0xabcd000000000000000000000000000000000000000000000000000000001234",
			[]*big.Int{big.NewInt(5_000_000), big.NewInt(0)}},
		{"3-outcome NegRisk", "0xfeed000000000000000000000000000000000000000000000000000000005678",
			[]*big.Int{big.NewInt(0), big.NewInt(0), big.NewInt(3_500_000)}},
		{"single-element amounts", "0x1111000000000000000000000000000000000000000000000000000000002222",
			[]*big.Int{big.NewInt(7_000_000)}},
		{"all zeros", "0x" + strings.Repeat("0", 64),
			[]*big.Int{big.NewInt(0), big.NewInt(0)}},
	}

	allMatch := true
	for _, c := range cases {
		manual := encodeNegRiskRedeemManual(c.conditionID, c.amounts)
		std, err := encodeNegRiskRedeemABI(c.conditionID, c.amounts)
		if err != nil {
			fmt.Printf("❌ %s: abi.Pack failed: %v\n", c.name, err)
			allMatch = false
			continue
		}

		match := hex.EncodeToString(manual) == hex.EncodeToString(std)
		mark := "✅"
		if !match {
			mark = "❌"
			allMatch = false
		}
		fmt.Printf("%s %s (len manual=%d std=%d)\n", mark, c.name, len(manual), len(std))
		if !match {
			fmt.Printf("   manual: %x\n", manual)
			fmt.Printf("   std:    %x\n", std)
			// 找第一个不同的 byte
			minLen := len(manual)
			if len(std) < minLen {
				minLen = len(std)
			}
			for i := 0; i < minLen; i++ {
				if manual[i] != std[i] {
					fmt.Printf("   first diff at byte %d (offset 0x%x): manual=%02x std=%02x\n", i, i, manual[i], std[i])
					break
				}
			}
		}
	}

	fmt.Println()
	if allMatch {
		fmt.Println("✅ 所有用例 SDK 手撸编码与 go-ethereum abi.Pack 完全一致")
		fmt.Println("→ ABI 编码不是 500 的锅，问题在 relayer 后端或 body 形态")
	} else {
		fmt.Println("❌ 发现编码差异，立即修 encodeNegRiskRedeem")
	}

	// 顺手打印一个典型 calldata，方便手动到 polygonscan input data decoder 验证
	fmt.Println()
	fmt.Println("Sample calldata (binary YES only, size=5):")
	sample := encodeNegRiskRedeemManual(cases[0].conditionID, cases[0].amounts)
	fmt.Printf("  to:       0xadA2005600Dec949baf300f4C6120000bDB6eAab (NegRiskCtfCollateralAdapter, 2026-05 迁移后)\n")
	fmt.Printf("  selector: 0x%x\n", sample[:4])
	fmt.Printf("  data:     0x%x\n", sample)
}
