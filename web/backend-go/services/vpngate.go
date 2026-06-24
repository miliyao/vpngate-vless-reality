// 负责请求 VPNGate API 镜像源，拉取、解析并过滤最优 VPN 节点
// 简体中文注释，包含高性能内存缓存以及 IP 排除退化降级机制

package services

import (
	"encoding/base64"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// VPNNode 对应 VPNGate API 的单条节点记录
type VPNNode struct {
	Hostname     string `json:"hostname"`
	IP           string `json:"ip"`
	Score        int64  `json:"score"`
	Ping         int    `json:"ping"`
	Speed        int64  `json:"speed"`
	CountryLong  string `json:"countryLong"`
	CountryShort string `json:"countryShort"`
	NumSessions  int    `json:"numSessions"`
	Uptime       int64  `json:"uptime"`
	TotalUsers   int64  `json:"totalUsers"`
	TotalTraffic string `json:"totalTraffic"`
	LogType      string `json:"logType"`
	Operator     string `json:"operator"`
	Message      string `json:"message"`
	OvpnB64      string `json:"-"` // Base64 编码的 .ovpn 配置
}

// OptimizedNode 最终优化配置输出的最优节点
type OptimizedNode struct {
	Hostname   string `json:"hostname"`
	IP         string `json:"ip"`
	Country    string `json:"country"`
	Ping       int    `json:"ping"`
	Speed      int64  `json:"speed"`
	OvpnConfig string `json:"ovpnConfig"`
}

var (
	// VPNGate 默认源列表
	vpngateURLs = []string{
		"https://www.vpngate.net/api/iphone/",
		"http://www2.vpngate.net/api/iphone/",
		"http://www.vpngate.net/api/iphone/",
	}

	timeout = 12 * time.Second

	// 内存缓存相关的线程安全管理
	cacheMutex    sync.RWMutex
	nodeCache     []VPNNode
	nodeCacheTime time.Time
	cacheTTL      = 1 * time.Hour
)

// FetchNodes 从镜像源中轮询拉取节点列表，带一小时内存缓存
func FetchNodes() ([]VPNNode, error) {
	cacheMutex.RLock()
	// 如果缓存未过期，直接返回内存缓存
	if len(nodeCache) > 0 && time.Since(nodeCacheTime) < cacheTTL {
		defer cacheMutex.RUnlock()
		fmt.Printf("[*] VPNGate 节点内存缓存命中（剩余有效时间 %d 分钟）\n", int((cacheTTL-time.Since(nodeCacheTime)).Minutes()))
		return nodeCache, nil
	}
	cacheMutex.RUnlock()

	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	// 双检锁，防高并发下重复穿透
	if len(nodeCache) > 0 && time.Since(nodeCacheTime) < cacheTTL {
		return nodeCache, nil
	}

	var data []byte
	var lastErr error

	// 简体中文注释：顺次循环拉取配置的多源镜像，直到成功或全部超时失败
	for i, url := range vpngateURLs {
		fmt.Printf("[*] 正在从 %s 获取 VPNGate 节点列表... (%d/%d)\n", url, i+1, len(vpngateURLs))
		client := &http.Client{Timeout: timeout}
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		// 伪装移动端 UA 防止限流
		req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X)")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			fmt.Printf("[!] 源 %s 获取节点失败: %v\n", url, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}

		// 简单判定包完整度，若符合标准则退出轮询
		if strings.Contains(string(body), "*vpn_servers") {
			data = body
			break
		}
		lastErr = errors.New("获取的数据格式不包含 *vpn_servers")
	}

	// 简体中文注释：若全部源抓取失败，退化为使用已过期的内存缓存兜底
	if len(data) == 0 {
		if len(nodeCache) > 0 {
			fmt.Println("[*] 启用降级方案：使用本地内存中已过期的缓存节点数据")
			return nodeCache, nil
		}
		return nil, fmt.Errorf("所有 VPNGate 镜像源均尝试获取失败，最后一次报错: %v", lastErr)
	}

	nodes, err := parseCSV(string(data))
	if err != nil {
		if len(nodeCache) > 0 {
			fmt.Printf("[!] 解析节点数据出错（%v），降级为使用旧内存缓存\n", err)
			return nodeCache, nil
		}
		return nil, err
	}

	// 更新缓存和时间戳
	nodeCache = nodes
	nodeCacheTime = time.Now()
	fmt.Printf("[+] 成功从 API 拉取并解析了 %d 个 VPNGate 节点\n", len(nodes))
	return nodes, nil
}

// 解析 VPNGate 导出的 CSV 数据段
func parseCSV(raw string) ([]VPNNode, error) {
	lines := strings.Split(raw, "\n")
	var csvBuilder strings.Builder
	isDataSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "*vpn_servers") {
			isDataSection = true
			continue
		}
		// 数据段结束标记，以 * 开头且正在数据段中
		if strings.HasPrefix(trimmed, "*") && isDataSection {
			break
		}
		if isDataSection {
			// 跳过带 # 的标题行
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			csvBuilder.WriteString(trimmed)
			csvBuilder.WriteString("\n")
		}
	}

	r := csv.NewReader(strings.NewReader(csvBuilder.String()))
	r.FieldsPerRecord = -1 // 允许每行字段数不一致，增强兼容性

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析错误: %v", err)
	}

	var nodes []VPNNode
	for _, cols := range records {
		if len(cols) < 15 {
			continue
		}
		score, _ := strconv.ParseInt(cols[2], 10, 64)
		ping, _ := strconv.Atoi(cols[3])
		speed, _ := strconv.ParseInt(cols[4], 10, 64)
		numSessions, _ := strconv.Atoi(cols[7])
		uptime, _ := strconv.ParseInt(cols[8], 10, 64)
		totalUsers, _ := strconv.ParseInt(cols[9], 10, 64)

		nodes = append(nodes, VPNNode{
			Hostname:     cols[0],
			IP:           cols[1],
			Score:        score,
			Ping:         ping,
			Speed:        speed,
			CountryLong:  cols[5],
			CountryShort: cols[6],
			NumSessions:  numSessions,
			Uptime:       uptime,
			TotalUsers:   totalUsers,
			TotalTraffic: cols[10],
			LogType:      cols[11],
			Operator:     cols[12],
			Message:      cols[13],
			OvpnB64:      cols[14],
		})
	}
	return nodes, nil
}

// GetBestNode 根据国家/地区获取排序最优的节点，支持退化降级处理
func GetBestNode(region string, excludeIps []string) (*OptimizedNode, error) {
	nodes, err := FetchNodes()
	if err != nil {
		return nil, err
	}

	// 转化为 map 便于 O(1) 判断 IP 是否需要排除
	excludeMap := make(map[string]bool)
	for _, ip := range excludeIps {
		excludeMap[ip] = true
	}

	// 1. 过滤对应国家且不属于排除列表的节点
	var filtered []VPNNode
	for _, n := range nodes {
		if strings.ToLower(n.CountryShort) == strings.ToLower(region) && !excludeMap[n.IP] {
			filtered = append(filtered, n)
		}
	}

	// 2. 简体中文注释：退化降级处理。若过滤失效 IP 后发现无匹配节点，但该地区实际上是有节点的，
	// 则自动清空排除条件，重新筛选该地区所有节点，允许尝试之前连过的唯一节点，防止自愈死锁。
	if len(filtered) == 0 && len(excludeIps) > 0 {
		var fallbackNodes []VPNNode
		for _, n := range nodes {
			if strings.ToLower(n.CountryShort) == strings.ToLower(region) {
				fallbackNodes = append(fallbackNodes, n)
			}
		}
		if len(fallbackNodes) > 0 {
			fmt.Printf("[!] 地区 %s 排除失效 IP 后无可用节点，降级为不进行 IP 过滤（可用节点数: %d）\n", region, len(fallbackNodes))
			filtered = fallbackNodes
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("未在 VPNGate 中找到国家/地区为 %s 的可用节点", region)
	}

	// 3. 排序算法：综合 Score（降序）与 Ping（升序）
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Score != filtered[j].Score {
			return filtered[i].Score > filtered[j].Score
		}
		return filtered[i].Ping < filtered[j].Ping
	})

	best := filtered[0]

	// 解码 Base64 获取 OVPN 配置文件
	ovpnDecoded, err := base64.StdEncoding.DecodeString(best.OvpnB64)
	if err != nil {
		return nil, fmt.Errorf("解码 OpenVPN 配置数据失败: %v", err)
	}

	ovpnConfig := string(ovpnDecoded)

	// 给 OpenVPN 配置文件注入稳定性优化参数
	if !strings.Contains(ovpnConfig, "keepalive") {
		ovpnConfig += "\nkeepalive 10 60\n"
	}
	if !strings.Contains(ovpnConfig, "resolv-retry") {
		ovpnConfig += "\nresolv-retry infinite\n"
	}
	if !strings.Contains(ovpnConfig, "auth-nocache") {
		ovpnConfig += "\nauth-nocache\n"
	}

	return &OptimizedNode{
		Hostname:   best.Hostname,
		IP:         best.IP,
		Country:    best.CountryShort,
		Ping:       best.Ping,
		Speed:      best.Speed,
		OvpnConfig: ovpnConfig,
	}, nil
}

// 种子初始化，用于随机打散源列表
func init() {
	rand.Seed(time.Now().UnixNano())
}
