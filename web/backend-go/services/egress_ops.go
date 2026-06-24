// 负责出口代理核心业务编排，管理端口分配、Xray/OpenVPN 配置文件写入、Docker 容器的拉起与物理销毁
// 简体中文注释，基于 Go 协程和 WaitGroup 实现并发控制为 3 的批量操作

package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vless-reality-panel/models"

	"github.com/google/uuid"
)

var ServerName = getEnv("RE_DOMAINS", "www.amd.com")

const (
	StartPort = 44301
	EndPort   = 44400
)

// CreateAllResult 批量创建出口的返回结构
type CreateAllResult struct {
	Created []map[string]interface{} `json:"created"`
	Errors  []map[string]interface{} `json:"errors"`
	Skipped []string                 `json:"skipped"`
}

// RebuildAllResult 批量重建出口的返回结构
type RebuildAllResult struct {
	Name   string `json:"name"`
	Region string `json:"region"`
	Port   int    `json:"port,omitempty"`
	Error  string `json:"error,omitempty"`
}

// DeleteAllResult 批量删除出口的返回结构
type DeleteAllResult struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// egressDir 获取出口的本地数据存储目录
func egressDir(name string) string {
	return filepath.Join("data", "egress", name)
}

// ensureDir 确保出口的数据目录存在
func ensureDir(name string) string {
	dir := egressDir(name)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// allocatePort 分配一个 44301-44400 之间未被占用的端口
func allocatePort() (int, error) {
	egresses, err := models.GetEgresses()
	if err != nil {
		return 0, err
	}

	usedPorts := make(map[int]bool)
	for _, e := range egresses {
		if e.Port > 0 {
			usedPorts[e.Port] = true
		}
	}

	for port := StartPort; port <= EndPort; port++ {
		if !usedPorts[port] {
			return port, nil
		}
	}

	return 0, fmt.Errorf("端口已耗尽 (%d-%d)", StartPort, EndPort)
}

// CreateEgress 创建单个出口代理，分配端口并启动 Docker 容器
func CreateEgress(name, region, clientUuid string) (*models.Egress, error) {
	name = strings.ToLower(name)
	region = strings.ToUpper(region)

	// 1. 从 VPNGate 获取最优节点
	node, err := GetBestNode(region, nil)
	if err != nil {
		return nil, fmt.Errorf("获取最优节点失败: %v", err)
	}

	// 2. 生成 X25519 Reality 密钥对
	keys, err := GenerateRealityKeys()
	if err != nil {
		return nil, fmt.Errorf("生成 Reality 密钥对失败: %v", err)
	}

	// 3. 分配端口
	port, err := allocatePort()
	if err != nil {
		return nil, err
	}

	// 4. 处理 UUID
	if clientUuid == "" {
		clientUuid = uuid.New().String()
	}

	// 5. 确保出口本地数据目录存在
	dir := ensureDir(name)

	// 6. 生成 Xray 配置文件写入本地
	destDomain := fmt.Sprintf("%s:443", ServerName)
	configPath := filepath.Join(dir, "config.json")
	err = GenerateXrayConfig(clientUuid, keys.PrivateKey, keys.ShortID, destDomain, ServerName, configPath)
	if err != nil {
		return nil, err
	}

	// 7. 写入 OpenVPN client.ovpn 文件
	ovpnPath := filepath.Join(dir, "client.ovpn")
	err = os.WriteFile(ovpnPath, []byte(node.OvpnConfig), 0644)
	if err != nil {
		return nil, fmt.Errorf("写入 client.ovpn 配置文件失败: %v", err)
	}

	// 8. 构造 Egress 结构体并存入 SQLite
	nowMs := time.Now().UnixNano() / 1e6
	egress := &models.Egress{
		Name:                name,
		Region:              region,
		Port:                port,
		UUID:                clientUuid,
		PrivateKey:          keys.PrivateKey,
		PublicKey:           keys.PublicKey,
		ShortId:             keys.ShortID,
		NodeIp:              node.IP,
		NodeHostname:        node.Hostname,
		Status:              "starting",
		Error:               "",
		CurrentEgressIp:     "",
		ContainerId:         "",
		CreatedAt:           nowMs,
		UpdatedAt:           nowMs,
		LastCheckTime:       nowMs,
		Latency:             node.Ping,
		FailureCount:        0,
		RebuildFailureCount: 0,
	}

	err = models.AddEgress(egress)
	if err != nil {
		return nil, fmt.Errorf("数据入库失败: %v", err)
	}

	// 9. 调用 Docker 服务拉起出口容器
	containerId, err := StartEgressContainer(egress)
	if err != nil {
		// 启动失败，更新状态为 error 并抛出异常
		_, _ = models.UpdateEgress(name, map[string]interface{}{
			"status": "error",
			"error":  fmt.Sprintf("启动 Docker 容器失败: %v", err),
		})
		return nil, fmt.Errorf("启动 Docker 容器失败: %v", err)
	}

	// 10. 更新容器 ID 且置为运行中
	updated, err := models.UpdateEgress(name, map[string]interface{}{
		"containerId": containerId,
		"status":      "running",
		"error":       "",
	})
	if err != nil {
		return nil, fmt.Errorf("更新容器运行时状态失败: %v", err)
	}

	return updated, nil
}

// RebuildEgress 重建出口（自愈的核心执行流程，若重建连续失败 3 次将自动销毁出口）
func RebuildEgress(name string) (*models.Egress, error) {
	name = strings.ToLower(name)
	egress, err := models.GetEgress(name)
	if err != nil {
		return nil, err
	}
	if egress == nil {
		return nil, fmt.Errorf("出口 %s 未找到", name)
	}

	// 置状态为重建中
	_, err = models.UpdateEgress(name, map[string]interface{}{
		"status": "rebuilding",
		"error":  "",
	})
	if err != nil {
		return nil, err
	}

	// 将当前失效节点的 IP 放入排除列表
	excludeIps := []string{}
	if egress.NodeIp != "" {
		excludeIps = append(excludeIps, egress.NodeIp)
	}

	// 1. 从 VPNGate 获取除失效 IP 之外的最佳新节点
	node, err := GetBestNode(egress.Region, excludeIps)
	if err != nil {
		// 重建抓取节点失败，处理重建失败累加及自动熔断删除
		newRebuildFailureCount := egress.RebuildFailureCount + 1
		if newRebuildFailureCount >= 3 {
			fmt.Printf("[-] 出口 %s 重建获取节点连续失败达上限 (%d/3)，触发自动销毁释放资源\n", name, newRebuildFailureCount)
			_ = DeleteEgress(name)
			return nil, fmt.Errorf("重建连续失败达上限，已自动删除该地区出口。原报错: %v", err)
		} else {
			_, _ = models.UpdateEgress(name, map[string]interface{}{
				"status":              "error",
				"error":               fmt.Sprintf("重建获取节点失败: %v", err),
				"rebuildFailureCount": newRebuildFailureCount,
			})
			return nil, err
		}
	}

	// 2. 写入最新的 client.ovpn 文件到本地
	dir := ensureDir(name)
	ovpnPath := filepath.Join(dir, "client.ovpn")
	err = os.WriteFile(ovpnPath, []byte(node.OvpnConfig), 0644)
	if err != nil {
		return nil, fmt.Errorf("重建写入 client.ovpn 失败: %v", err)
	}

	// 3. 先更新节点信息，再通过 StartEgressContainer 进行 Docker 重启/新建
	_, err = models.UpdateEgress(name, map[string]interface{}{
		"nodeIp":       node.IP,
		"nodeHostname": node.Hostname,
		"latency":      node.Ping,
	})
	if err != nil {
		return nil, err
	}

	latestEgress, err := models.GetEgress(name)
	if err != nil {
		return nil, err
	}

	// 4. 重启 Docker 出口容器
	containerId, err := StartEgressContainer(latestEgress)
	if err != nil {
		newRebuildFailureCount := egress.RebuildFailureCount + 1
		if newRebuildFailureCount >= 3 {
			fmt.Printf("[-] 出口 %s 容器启动连续失败达上限 (%d/3)，触发自动销毁\n", name, newRebuildFailureCount)
			_ = DeleteEgress(name)
			return nil, fmt.Errorf("重建连续失败达上限，已自动删除该地区出口。原报错: %v", err)
		} else {
			_, _ = models.UpdateEgress(name, map[string]interface{}{
				"status":              "error",
				"error":               fmt.Sprintf("重启出口容器失败: %v", err),
				"rebuildFailureCount": newRebuildFailureCount,
			})
			return nil, err
		}
	}

	// 5. 成功完成重建，重置故障计数器
	updated, err := models.UpdateEgress(name, map[string]interface{}{
		"containerId":         containerId,
		"status":              "running",
		"error":               "",
		"failureCount":        0,
		"rebuildFailureCount": 0,
	})
	if err != nil {
		return nil, err
	}

	return updated, nil
}

// DeleteEgress 物理销毁出口容器，清空物理文件和 SQLite 记录
func DeleteEgress(name string) error {
	name = strings.ToLower(name)

	// 1. 停止并移除 Docker 容器
	_ = StopAndRemoveContainer(name)

	// 2. 物理删除出口配置文件夹（带前缀安全校验）
	dir := egressDir(name)
	targetParent, _ := filepath.Abs(filepath.Join("data", "egress"))
	absDir, _ := filepath.Abs(dir)
	if strings.HasPrefix(absDir, targetParent) && absDir != targetParent {
		if _, err := os.Stat(absDir); err == nil {
			_ = os.RemoveAll(absDir)
		}
	}

	// 3. 从数据库中彻底删除
	return models.DeleteEgress(name)
}

// CreateAll 批量并发创建指定地区的出口
func CreateAll(regions []string, uuidStr string) (*CreateAllResult, error) {
	if uuidStr == "" {
		uuidStr = uuid.New().String()
	}

	nodes, err := FetchNodes()
	if err != nil {
		return nil, err
	}

	existingEgresses, err := models.GetEgresses()
	if err != nil {
		return nil, err
	}

	existingRegions := make(map[string]bool)
	for _, e := range existingEgresses {
		existingRegions[strings.ToUpper(e.Region)] = true
	}

	// 汇总每个国家分数最高的节点记录
	regionMap := make(map[string]VPNNode)
	for _, n := range nodes {
		code := strings.ToUpper(n.CountryShort)
		if len(code) != 2 {
			continue
		}
		existing, ok := regionMap[code]
		if !ok || n.Score > existing.Score {
			regionMap[code] = n
		}
	}

	var targetRegions []string
	if len(regions) > 0 {
		for _, r := range regions {
			code := strings.ToUpper(r)
			if _, ok := regionMap[code]; ok {
				targetRegions = append(targetRegions, code)
			}
		}
	} else {
		for code := range regionMap {
			targetRegions = append(targetRegions, code)
		}
	}

	var newRegions []string
	var skipped []string
	for _, r := range targetRegions {
		if existingRegions[r] {
			skipped = append(skipped, r)
		} else {
			newRegions = append(newRegions, r)
		}
	}

	var created []map[string]interface{}
	var errs []map[string]interface{}

	var createdMutex sync.Mutex
	var errsMutex sync.Mutex

	var wg sync.WaitGroup
	limitCh := make(chan struct{}, 3) // 并发限制为 3

	for _, region := range newRegions {
		wg.Add(1)
		limitCh <- struct{}{}
		go func(r string) {
			defer wg.Done()
			defer func() { <-limitCh }()

			name := strings.ToLower(r)
			egress, err := CreateEgress(name, r, uuidStr)
			if err != nil {
				errsMutex.Lock()
				errs = append(errs, map[string]interface{}{
					"region": r,
					"error":  err.Error(),
				})
				errsMutex.Unlock()
			} else {
				createdMutex.Lock()
				created = append(created, map[string]interface{}{
					"name":   egress.Name,
					"region": egress.Region,
					"port":   egress.Port,
					"uuid":   egress.UUID,
				})
				createdMutex.Unlock()
			}
		}(region)
	}

	wg.Wait()

	return &CreateAllResult{
		Created: created,
		Errors:  errs,
		Skipped: skipped,
	}, nil
}

// RebuildAll 批量重建所有现存出口，支持并发限制为 3
func RebuildAll() ([]RebuildAllResult, error) {
	list, err := models.GetEgresses()
	if err != nil {
		return nil, err
	}

	var results []RebuildAllResult
	var resultsMutex sync.Mutex

	var wg sync.WaitGroup
	limitCh := make(chan struct{}, 3)

	for _, e := range list {
		wg.Add(1)
		limitCh <- struct{}{}
		go func(eg models.Egress) {
			defer wg.Done()
			defer func() { <-limitCh }()

			res, err := RebuildEgress(eg.Name)
			resultsMutex.Lock()
			if err != nil {
				results = append(results, RebuildAllResult{
					Name:   eg.Name,
					Region: eg.Region,
					Error:  err.Error(),
				})
			} else {
				results = append(results, RebuildAllResult{
					Name:   res.Name,
					Region: res.Region,
					Port:   res.Port,
				})
			}
			resultsMutex.Unlock()
		}(e)
	}

	wg.Wait()
	return results, nil
}

// DeleteAll 批量销毁所有现存出口
func DeleteAll() ([]DeleteAllResult, error) {
	list, err := models.GetEgresses()
	if err != nil {
		return nil, err
	}

	var results []DeleteAllResult
	var resultsMutex sync.Mutex

	var wg sync.WaitGroup
	limitCh := make(chan struct{}, 3)

	for _, e := range list {
		wg.Add(1)
		limitCh <- struct{}{}
		go func(eg models.Egress) {
			defer wg.Done()
			defer func() { <-limitCh }()

			err := DeleteEgress(eg.Name)
			resultsMutex.Lock()
			if err != nil {
				results = append(results, DeleteAllResult{
					Name:  eg.Name,
					Error: err.Error(),
				})
			} else {
				results = append(results, DeleteAllResult{
					Name: eg.Name,
				})
			}
			resultsMutex.Unlock()
		}(e)
	}

	wg.Wait()
	return results, nil
}

// BuildLink 根据 VPS IP / 域名和端口生成客户端使用的 VLESS Reality 订阅链接
func BuildLink(egress *models.Egress, vpsHost string) string {
	return fmt.Sprintf("vless://%s@%s:%d?encryption=none&type=tcp&security=reality&flow=xtls-rprx-vision&pbk=%s&sid=%s&sni=%s&fp=chrome&spx=%%2F&headerType=none#%s",
		egress.UUID, vpsHost, egress.Port, egress.PublicKey, egress.ShortId, ServerName, egress.Name)
}
