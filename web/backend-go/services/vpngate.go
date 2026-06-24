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
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"vless-reality-panel/models"
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
	Hostname       string  `json:"hostname"`
	IP             string  `json:"ip"`
	Country        string  `json:"country"`
	Ping           int     `json:"ping"`
	Speed          int64   `json:"speed"`
	Score          int64   `json:"score"`
	Sessions       int     `json:"sessions"`
	Uptime         int64   `json:"uptime"`
	Proto          string  `json:"proto"`
	RemoteHost     string  `json:"remoteHost"`
	RemotePort     string  `json:"remotePort"`
	SelectionScore float64 `json:"selectionScore"`
	OvpnConfig     string  `json:"ovpnConfig"`
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

const (
	defaultCandidateLimit      = 8
	defaultFailureCooldownMs   = 30 * 60 * 1000
	maxSelectionFailurePenalty = 20.0
)

// FetchNodes 从镜像源中轮询拉取节点列表，带一小时内存缓存
func FetchNodes() ([]VPNNode, error) {
	cacheMutex.RLock()
	// 如果缓存未过期，直接返回内存缓存
	if len(nodeCache) > 0 && time.Since(nodeCacheTime) < cacheTTL {
		defer cacheMutex.RUnlock()
		fmt.Printf("[*] VPNGate 节点内存缓存命中（剩余有效时间 %d 分钟）\n", int((cacheTTL - time.Since(nodeCacheTime)).Minutes()))
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
	candidates, err := GetCandidateNodes(region, excludeIps, 1)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("未在 VPNGate 中找到国家/地区为 %s 的可用节点", region)
	}
	return &candidates[0], nil
}

// GetCandidateNodes 根据国家/地区返回按综合质量排序的 VPNGate 候选节点
func GetCandidateNodes(region string, excludeIps []string, limit int) ([]OptimizedNode, error) {
	nodes, err := FetchNodes()
	if err != nil {
		return nil, err
	}

	region = strings.ToUpper(region)
	if limit <= 0 {
		limit = defaultCandidateLimit
	}

	failureCooldownMs := int64(getEnvInt("VPNGATE_FAILURE_COOLDOWN_MS", defaultFailureCooldownMs))
	recentFailureSince := time.Now().UnixNano()/1e6 - failureCooldownMs
	recentFailedIPs, err := models.GetRecentFailedVPNGateIPs(region, recentFailureSince)
	if err != nil {
		fmt.Printf("[!] 获取 VPNGate 节点失败历史失败，将跳过近期失败排除: %v\n", err)
	}

	// 转化为 map 便于 O(1) 判断 IP 是否需要排除。显式排除永远保留，近期失败排除允许在无节点时降级。
	excludeMap := make(map[string]bool)
	for _, ip := range excludeIps {
		excludeMap[ip] = true
	}
	for _, ip := range recentFailedIPs {
		excludeMap[ip] = true
	}

	filtered := filterVPNGateNodes(nodes, region, excludeMap)

	// 若近期失败冷却导致无候选，先只取消近期失败排除，保留当前失效 IP 的显式排除。
	if len(filtered) == 0 && len(recentFailedIPs) > 0 {
		explicitOnly := make(map[string]bool)
		for _, ip := range excludeIps {
			explicitOnly[ip] = true
		}
		filtered = filterVPNGateNodes(nodes, region, explicitOnly)
		if len(filtered) > 0 {
			fmt.Printf("[!] 地区 %s 近期失败 IP 排除后无节点，已降级允许冷却节点参与候选（候选数: %d）\n", region, len(filtered))
		}
	}

	// 若连显式排除后也无候选，最终取消 IP 过滤，避免单节点地区自愈死锁。
	if len(filtered) == 0 && len(excludeIps) > 0 {
		filtered = filterVPNGateNodes(nodes, region, map[string]bool{})
		if len(filtered) > 0 {
			fmt.Printf("[!] 地区 %s 排除当前失效 IP 后无节点，已降级允许原 IP 重试（候选数: %d）\n", region, len(filtered))
		}
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("未在 VPNGate 中找到国家/地区为 %s 的可用节点", region)
	}

	history, err := models.GetVPNGateHistoryMap(region)
	if err != nil {
		fmt.Printf("[!] 获取 VPNGate 节点历史评分失败，将仅使用实时字段评分: %v\n", err)
		history = map[string]models.VPNGateNodeHistory{}
	}

	maxScore, maxSpeed, maxUptime, maxSessions := candidateMaxima(filtered)
	type scoredNode struct {
		node           VPNNode
		selectionScore float64
	}
	scored := make([]scoredNode, 0, len(filtered))
	for _, n := range filtered {
		scored = append(scored, scoredNode{
			node:           n,
			selectionScore: scoreVPNGateNode(n, history[n.IP], maxScore, maxSpeed, maxUptime, maxSessions),
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].selectionScore != scored[j].selectionScore {
			return scored[i].selectionScore > scored[j].selectionScore
		}
		if scored[i].node.Score != scored[j].node.Score {
			return scored[i].node.Score > scored[j].node.Score
		}
		return scored[i].node.Ping < scored[j].node.Ping
	})

	var candidates []OptimizedNode
	for _, item := range scored {
		node, err := buildOptimizedNode(item.node, item.selectionScore)
		if err != nil {
			_ = models.RecordVPNGateNodeFailure(item.node.IP, region, item.node.Hostname, fmt.Sprintf("OpenVPN 配置解码失败: %v", err))
			continue
		}
		candidates = append(candidates, *node)
		if len(candidates) >= limit {
			break
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("地区 %s 的 VPNGate 候选节点均无法解码 OpenVPN 配置", region)
	}
	return candidates, nil
}

func filterVPNGateNodes(nodes []VPNNode, region string, excludeMap map[string]bool) []VPNNode {
	var filtered []VPNNode
	for _, n := range nodes {
		if strings.ToUpper(n.CountryShort) != region {
			continue
		}
		if n.IP == "" || net.ParseIP(n.IP) == nil {
			continue
		}
		if excludeMap[n.IP] {
			continue
		}
		if strings.TrimSpace(n.OvpnB64) == "" {
			continue
		}
		filtered = append(filtered, n)
	}
	return filtered
}

func candidateMaxima(nodes []VPNNode) (int64, int64, int64, int) {
	var maxScore, maxSpeed, maxUptime int64
	var maxSessions int
	for _, n := range nodes {
		if n.Score > maxScore {
			maxScore = n.Score
		}
		if n.Speed > maxSpeed {
			maxSpeed = n.Speed
		}
		if n.Uptime > maxUptime {
			maxUptime = n.Uptime
		}
		if n.NumSessions > maxSessions {
			maxSessions = n.NumSessions
		}
	}
	return maxScore, maxSpeed, maxUptime, maxSessions
}

func ratioInt64(value, max int64) float64 {
	if value <= 0 || max <= 0 {
		return 0
	}
	return float64(value) / float64(max)
}

func scoreVPNGateNode(n VPNNode, history models.VPNGateNodeHistory, maxScore, maxSpeed, maxUptime int64, maxSessions int) float64 {
	scorePart := ratioInt64(n.Score, maxScore) * 35
	speedPart := ratioInt64(n.Speed, maxSpeed) * 20
	uptimePart := ratioInt64(n.Uptime, maxUptime) * 15

	pingPart := 0.0
	if n.Ping > 0 {
		pingPart = (1 / (1 + float64(n.Ping)/300)) * 15
	}

	sessionPart := 5.0
	if maxSessions > 0 {
		sessionPart = (1 - float64(n.NumSessions)/float64(maxSessions)) * 10
		if sessionPart < 0 {
			sessionPart = 0
		}
	}

	historyPart := 0.0
	totalHistory := history.SuccessCount + history.FailureCount
	if totalHistory > 0 {
		historyPart = (float64(history.SuccessCount) / float64(totalHistory)) * 5
	}
	failurePenalty := float64(history.FailureCount) * 3
	if failurePenalty > maxSelectionFailurePenalty {
		failurePenalty = maxSelectionFailurePenalty
	}

	return scorePart + speedPart + uptimePart + pingPart + sessionPart + historyPart - failurePenalty
}

func buildOptimizedNode(best VPNNode, selectionScore float64) (*OptimizedNode, error) {
	ovpnDecoded, err := base64.StdEncoding.DecodeString(best.OvpnB64)
	if err != nil {
		return nil, err
	}

	ovpnConfig := string(ovpnDecoded)
	proto, remoteHost, remotePort := parseOpenVPNMetadata(ovpnConfig)

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
		Hostname:       best.Hostname,
		IP:             best.IP,
		Country:        best.CountryShort,
		Ping:           best.Ping,
		Speed:          best.Speed,
		Score:          best.Score,
		Sessions:       best.NumSessions,
		Uptime:         best.Uptime,
		Proto:          proto,
		RemoteHost:     remoteHost,
		RemotePort:     remotePort,
		SelectionScore: selectionScore,
		OvpnConfig:     ovpnConfig,
	}, nil
}

func parseOpenVPNMetadata(config string) (string, string, string) {
	var proto, remoteHost, remotePort string
	lines := strings.Split(config, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "proto":
			if len(fields) >= 2 && proto == "" {
				proto = fields[1]
			}
		case "remote":
			if len(fields) >= 2 && remoteHost == "" {
				remoteHost = fields[1]
				if len(fields) >= 3 {
					remotePort = fields[2]
				}
			}
		}
	}
	return proto, remoteHost, remotePort
}

// 种子初始化，用于随机打散源列表
func init() {
	rand.Seed(time.Now().UnixNano())
}
