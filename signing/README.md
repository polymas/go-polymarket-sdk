# Signing Package

签名包提供了以太坊签名功能，包括 EIP-712 签名和 HMAC 签名。

## 使用 Signer

`Signer` 是主要的签名器类型，可以从私钥创建并用于各种签名操作。

### 导入

```go
import "github.com/polymas/go-polymarket-sdk/signing"
```

### 创建 Signer

```go
// 从私钥创建 Signer
privateKey := "0x1234567890abcdef..." // 您的私钥（带或不带 0x 前缀）
chainID := types.ChainID(137) // Polygon Mainnet

signer, err := signing.NewSigner(privateKey, chainID)
if err != nil {
    log.Fatal(err)
}

// 获取地址
address := signer.Address()
fmt.Printf("Address: %s\n", address)

// 获取链ID
chainID := signer.ChainID()
```

### 签名操作

```go
// 1. 对消息哈希进行签名（返回64字节，不包含恢复ID）
messageHash := common.HexToHash("0x...")
signature, err := signer.Sign(messageHash)

// 2. 对消息哈希进行签名并返回包含v值的完整签名（65字节）
signatureWithRecovery, err := signer.SignWithRecovery(messageHash)

// 3. 对原始字节进行签名
data := []byte("Hello, World!")
signature, err := signer.SignBytes(data)

// 4. 对哈希进行签名并返回十六进制字符串格式
hash := common.HexToHash("0x...")
signatureHex, err := signer.SignHash(hash)

// 5. 对以太坊消息进行签名（EIP-191）
message := []byte("Hello, World!")
signature, err := signer.SignMessage(message)
```

### CLOB 认证消息签名

```go
// 对 CLOB 认证消息进行 EIP-712 签名
timestamp := time.Now().Unix()
nonce := 12345
signature, err := signing.SignClobAuthMessage(signer, timestamp, nonce)
```

### HMAC 签名

```go
// 使用 API 凭证创建 HMAC 签名
secret := "your-base64-secret"
timestamp := strconv.FormatInt(time.Now().Unix(), 10)
method := "POST"
requestPath := "/submit"
body := `{"key": "value"}`

signature, err := signing.BuildHMACSignature(secret, timestamp, method, requestPath, body)
```

### 清理敏感信息

```go
// 在不再需要 Signer 时，清理内存中的私钥
signer.Clear()
// 注意：清理后 Signer 将无法再使用
```

## 错误处理

```go
var (
    ErrInvalidPrivateKey = errors.New("invalid private key")
    ErrInvalidPublicKey  = errors.New("invalid public key")
    ErrSigningFailed     = errors.New("signing failed")
)
```

## 完整示例

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/polymas/go-polymarket-sdk/signing"
    "github.com/polymas/go-polymarket-sdk/types"
)

func main() {
    // 创建 Signer
    privateKey := "0x1234567890abcdef..."
    chainID := types.ChainID(137)
    
    signer, err := signing.NewSigner(privateKey, chainID)
    if err != nil {
        log.Fatal(err)
    }
    
    // 获取地址
    address := signer.Address()
    fmt.Printf("Address: %s\n", address)
    
    // 签名消息
    message := []byte("Hello, Polymarket!")
    signature, err := signer.SignMessage(message)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Signature: 0x%x\n", signature)
    
    // 清理
    defer signer.Clear()
}
```
