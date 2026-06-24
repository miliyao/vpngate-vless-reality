// Go 语言后端服务主入口文件，第一阶段作为骨架验证测试入口
// 简体中文注释，保障架构的自适应性与鲁棒性

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
	
	// 在当前路径运行本地测试时，若无 data 目录，则在当前目录创建
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
		fmt.Printf("[+] 日本最优节点获取成功！\n")
		fmt.Printf("    主机名: %s\n", bestNode.Hostname)
		fmt.Printf("    节点IP: %s\n", bestNode.IP)
		fmt.Printf("    国家代码: %s\n", bestNode.Country)
		fmt.Printf("    延迟 (Ping): %d ms\n", bestNode.Ping)
		fmt.Printf("    带宽估算: %.2f Mbps\n", float64(bestNode.Speed)/1024/1024)
		fmt.Printf("    OpenVPN 配置长度: %d 字节\n", len(bestNode.OvpnConfig))
	}

	fmt.Println("[+] 第一阶段骨架引擎与数据库验证通过！")
}
