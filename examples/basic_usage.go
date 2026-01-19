package main

import (
	"fmt"
	"log"
	"time"

	"github.com/polymas/go-polymarket-sdk/config"
	"github.com/polymas/go-polymarket-sdk/container"
	"github.com/polymas/go-polymarket-sdk/types"
	"github.com/polymas/go-polymarket-sdk/web3"
)

// 示例：使用新架构的基本用法
func main() {
	// 1. 创建配置
	cfg := config.NewConfig(
		config.WithChainID(types.Polygon),
		config.WithSignatureType(types.ProxySignatureType),
		config.WithHTTPTimeout(30*time.Second),
		config.WithMaxRetries(3),
		config.WithCacheEnabled(true),
		config.WithLogLevel("INFO"),
	)

	// 2. 创建依赖注入容器
	container := container.NewContainer(cfg)
	defer container.Close()

	// 3. 初始化 Web3 客户端（示例）
	// 注意：实际使用时需要提供真实的私钥
	privateKey := "your-private-key-here"
	web3Client, err := web3.NewClient(
		privateKey,
		cfg.Web3.SignatureType,
		cfg.Web3.ChainID,
	)
	if err != nil {
		log.Fatalf("创建 Web3 客户端失败: %v", err)
	}
	container.SetWeb3Client(web3Client)

	// 4. 使用容器中的依赖
	cache := container.GetCache()

	// 设置缓存值
	cache.Set("example:key", "example:value", 5*time.Minute)

	// 获取缓存值
	value, ok := cache.Get("example:key")
	if ok {
		fmt.Printf("从缓存获取值: %v\n", value)
	}

	// 5. 获取配置
	config := container.GetConfig()
	fmt.Printf("CLOB API 域名: %s\n", config.API.ClobDomain)
	fmt.Printf("链 ID: %d\n", config.Web3.ChainID)
	fmt.Printf("HTTP 超时: %v\n", config.HTTP.Timeout)

	// 6. 获取 Web3 客户端
	client := container.GetWeb3Client()
	if client != nil {
		fmt.Printf("Web3 客户端地址: %s\n", client.GetBaseAddress())
	}

	fmt.Println("示例执行完成！")
}
