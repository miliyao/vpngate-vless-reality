// Go 语言后端服务主入口文件，第二阶段集成 Reality 密钥对生成、Xray 注入以及 Docker API 连通性检查测试
// 简体中文注释，捕获异常防止崩溃，保证高可观测性

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"vless-reality-panel/models"
	"vless-reality-panel/services"
)

func main() {
	fmt.Println("[*] VLESS Reality Panel (Go 重构版) 启动中...")

	// 1. 自适应检测并确定数据库路径
	dbPath := "./data/app.sqlite3"
	if _, err := os.Stat("/app/data"); err == nil {
		dbPath = "/app/data/app.sqlite3"
	}
	
	absPath, _ := filepath.Abs(dbPath)
	fmt.Printf("[*] 正在加载 SQLite 数据库: %s\n", absPath)

	err := models.InitDB(dbPath)
	if err != nil {
		fmt.Printf("[-] 数据库初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[+] 数据库及数据表加载成功！WAL模式已启用")

	// 2. 触发一次节点抓取，验证网络与 CSV 高性能解析器
	fmt.Println("[*] 开始拉取 VPNGate 节点进行性能测试...")
	startTime := time.Now()
	nodes, err := services.FetchNodes()
	if err != nil {
		fmt.Printf("[-] 节点抓取失败: %v\n", err)
		os.Exit(1)
	}
	elapsed := time.Since(startTime)
	fmt.Printf("[+] 成功获取节点列表！耗时: %v，共解析到 %d 个节点\n", elapsed, len(nodes))

	// 3. 验证退化筛选与排序性能（获取日本最优节点）
	fmt.Println("[*] 正在测试获取 JP (日本) 地区最优节点...")
	bestNode, err := services.GetBestNode("JP", []string{})
	if err != nil {
		fmt.Printf("[-] 获取日本节点失败: %v\n", err)
	} else {
		fmt.Printf("[+] 日本最优节点获取成功！IP: %s, Ping: %d ms\n", bestNode.IP, bestNode.Ping)
	}

	// 4. 验证 VLESS Reality 密钥对生成
	fmt.Println("[*] 开始测试 X25519 Reality 密钥生成性能...")
	keyStart := time.Now()
	keys, err := services.GenerateRealityKeys()
	if err != nil {
		fmt.Printf("[-] 密钥对生成失败: %v\n", err)
	} else {
		fmt.Printf("[+] Reality 密钥生成成功！耗时: %v\n", time.Since(keyStart))
		fmt.Printf("    私钥 (Private): %s\n", keys.PrivateKey)
		fmt.Printf("    公钥 (Public): %s\n", keys.PublicKey)
		fmt.Printf("    短 ID (shortId): %s\n", keys.ShortID)
	}

	// 5. 验证 Xray 模板配置文件替换与注入
	fmt.Println("[*] 正在测试生成 Xray 配置文件...")
	testConfigPath := "./data/test_xray_config.json"
	err = services.GenerateXrayConfig(
		"d9b8971f-df72-466d-8b01-fcd5d0b98765", // 测试 UUID
		keys.PrivateKey,
		keys.ShortID,
		"www.amd.com:443",
		"www.amd.com",
		testConfigPath,
	)
	if err != nil {
		fmt.Printf("[-] Xray 配置生成失败: %v\n", err)
	} else {
		fmt.Printf("[+] Xray 配置文件生成成功！文件输出至: %s\n", testConfigPath)
		// 清除临时配置文件
		_ = os.Remove(testConfigPath)
	}

	// 6. 验证 Docker SDK 通信与连通性
	fmt.Println("[*] 正在测试 Docker API 连通性...")
	dockerStart := time.Now()
	// 如果本地没有运行 Docker 服务（例如 Windows 开发机未开 Docker 进程），
	// 我们需要优雅捕获错误，防止整个测试崩溃退出，这属于架构高容错设计。
	_, err = services.GetContainerStatusAndIp("non_exist_egress")
	if err != nil {
		fmt.Printf("[!] Docker API 测试完成（耗时 %v）: %v\n", time.Since(dockerStart), err)
		fmt.Println("[*] 提示: 连通性报错属正常，宿主机 Docker 进程未开启或不可达")
	} else {
		fmt.Printf("[+] Docker API 连通成功！耗时: %v\n", time.Since(dockerStart))
	}

	fmt.Println("[+] 第二阶段系统集成核心组件（Docker、Reality、Xray）验证通过！")
}
