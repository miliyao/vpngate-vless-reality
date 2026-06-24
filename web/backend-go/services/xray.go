// 负责根据配置文件模板生成各出口容器所需的 Xray 配置文件
// 简体中文注释，模块级缓存，文件路径自适应遍历

package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	cachedTemplate string
	templateOnce   sync.Once
	templateErr    error
)

// 遍历常见路径自适应定位模板文件
func resolveTemplatePath() string {
	candidates := []string{
		"/app/config/xray-config.template.json",
		"./config/xray-config.template.json",
		"../config/xray-config.template.json",
		"../../config/xray-config.template.json",
		"../../../config/xray-config.template.json",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "./config/xray-config.template.json"
}

// 加载并模块级缓存模板内容
func loadTemplate() (string, error) {
	templateOnce.Do(func() {
		tplPath := resolveTemplatePath()
		bytes, err := os.ReadFile(tplPath)
		if err != nil {
			templateErr = fmt.Errorf("读取模板文件 %s 失败: %v", tplPath, err)
			return
		}
		cachedTemplate = string(bytes)
		fmt.Printf("[*] 成功加载并缓存 Xray 配置文件模板: %s\n", tplPath)
	})
	return cachedTemplate, templateErr
}

// GenerateXrayConfig 生成 Xray 配置文件并写入指定输出路径
func GenerateXrayConfig(uuid, privateKey, shortID, destDomain, serverName, outputPath string) error {
	templateContent, err := loadTemplate()
	if err != nil {
		return err
	}

	// 容错默认值
	if destDomain == "" {
		destDomain = "www.amd.com:443"
	}
	if serverName == "" {
		serverName = "www.amd.com"
	}

	// 简体中文注释：使用字符串替换注入客户端 UUID、Reality 私钥和 shortId
	content := templateContent
	content = strings.ReplaceAll(content, "{{UUID}}", uuid)
	content = strings.ReplaceAll(content, "{{PRIVATE_KEY}}", privateKey)
	content = strings.ReplaceAll(content, "{{SHORT_ID}}", shortID)
	content = strings.ReplaceAll(content, "{{DEST_DOMAIN}}", destDomain)
	content = strings.ReplaceAll(content, "{{SERVER_NAME}}", serverName)

	// 确保输出目录存在
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("创建输出目录 %s 失败: %v", outputDir, err)
	}

	// 写入目标配置文件
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入 Xray 配置文件失败: %v", err)
	}

	fmt.Printf("[+] Xray 配置文件生成成功: %s\n", outputPath)
	return nil
}
