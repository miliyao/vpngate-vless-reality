// 负责生成 VLESS Reality 使用的 X25519 密钥对与 shortId
// 简体中文注释，基于 Go 标准库实现，不依赖外部可执行文件以达到高性能与跨平台

package services

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// RealityKeys 包含生成的密钥对和 shortId
type RealityKeys struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
	ShortID    string `json:"shortId"`
}

// GenerateRealityKeys 使用 crypto/ecdh 原生生成 X25519 密钥，并提供 URL 安全的 Base64 编码
func GenerateRealityKeys() (*RealityKeys, error) {
	// 1. 生成 X25519 密钥对
	privKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成 X25519 密钥失败: %v", err)
	}

	// 2. 导出为原始的 32 字节私钥与公钥
	rawPrivate := privKey.Bytes()
	rawPublic := privKey.PublicKey().Bytes()

	// 3. 转成 URL 安全的 Base64 编码（VLESS Reality 标准，不带等号填充）
	privateKeyBase64 := base64.RawURLEncoding.EncodeToString(rawPrivate)
	publicKeyBase64 := base64.RawURLEncoding.EncodeToString(rawPublic)

	// 4. 生成 8 字节随机数，并转换为 16 位十六进制的 shortId
	shortBytes := make([]byte, 8)
	if _, err := rand.Read(shortBytes); err != nil {
		return nil, fmt.Errorf("生成 shortId 失败: %v", err)
	}
	shortID := hex.EncodeToString(shortBytes)

	return &RealityKeys{
		PrivateKey: privateKeyBase64,
		PublicKey:  publicKeyBase64,
		ShortID:    shortID,
	}, nil
}
