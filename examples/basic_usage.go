package main

import (
	"fmt"
	"log"

	"github.com/polymas/go-polymarket-sdk/clob"
	"github.com/polymas/go-polymarket-sdk/gamma"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

func main() {
	// ========== 方式1: 使用只读客户端（不需要私钥） ==========
	fmt.Println("=== 使用只读客户端（不需要私钥）===")

	// 创建只读 CLOB 客户端，可以查询市场数据，但不能进行交易操作
	readonlyClient := clob.NewReadonlyClient()

	// 使用只读客户端查询订单簿（不需要私钥）
	orderBook, err := readonlyClient.GetOrderBook("0x1234567890123456789012345678901234567890")
	if err != nil {
		log.Printf("获取订单簿失败: %v", err)
	} else {
		fmt.Printf("订单簿: %+v\n", orderBook)
	}

	// 使用只读客户端查询中间价
	midpoint, err := readonlyClient.GetMidpoint("0x1234567890123456789012345678901234567890")
	if err != nil {
		log.Printf("获取中间价失败: %v", err)
	} else {
		fmt.Printf("中间价: %+v\n", midpoint)
	}

	// ========== 方式2: 使用完整客户端（需要私钥） ==========
	fmt.Println("\n=== 使用完整客户端（需要私钥）===")

	// 1. 创建 Web3 客户端（需要私钥）
	privateKey := "your-private-key-here" // 替换为你的私钥
	web3Client, err := web3.NewClient(
		privateKey,
		types.ProxySignatureType, // 或 types.SafeSignatureType
		types.Polygon,            // 或 types.Amoy (测试网)
	)
	if err != nil {
		log.Printf("创建 Web3 客户端失败: %v (跳过完整客户端示例)", err)
		return
	}
	defer web3Client.Close()

	fmt.Printf("Web3 客户端地址: %s\n", web3Client.GetBaseAddress())

	// 2. 创建完整 CLOB 客户端（需要 Web3 客户端）
	clobClient, err := clob.NewClient(web3Client)
	if err != nil {
		log.Fatalf("创建 CLOB 客户端失败: %v", err)
	}

	// 完整客户端可以使用所有功能，包括只读接口
	orderBook2, err := clobClient.GetOrderBook("0x1234567890123456789012345678901234567890")
	if err != nil {
		log.Printf("获取订单簿失败: %v", err)
	} else {
		fmt.Printf("订单簿: %+v\n", orderBook2)
	}

	// 3. 创建 Gamma 客户端（不需要认证，可以直接使用）
	gammaClient := gamma.NewClient()

	// 使用 Gamma 客户端查询市场
	market, err := gammaClient.GetMarket("0x1234567890123456789012345678901234567890")
	if err != nil {
		log.Printf("获取市场失败: %v", err)
	} else {
		fmt.Printf("市场: %+v\n", market)
	}

	// 4. 查询 USDC 余额
	balance, err := clobClient.GetUSDCBalance()
	if err != nil {
		log.Printf("获取 USDC 余额失败: %v", err)
	} else {
		fmt.Printf("USDC 余额: %.6f\n", balance)
	}

	// 5. 查询 Web3 余额
	polBalance, err := web3Client.GetPOLBalance()
	if err != nil {
		log.Printf("获取 POL 余额失败: %v", err)
	} else {
		fmt.Printf("POL 余额: %.6f\n", polBalance)
	}

	fmt.Println("示例执行完成！")
}
