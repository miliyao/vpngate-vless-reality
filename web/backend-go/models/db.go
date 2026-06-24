// 负责持久化核心模型层，使用纯 Go SQLite 驱动支持无 CGO 编译
// 简体中文注释，符合中国开发者的命名与技术习惯

package models

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// Egress 出口代理模型结构体
type Egress struct {
	Name                string `json:"name"`
	Region              string `json:"region"`
	Port                int    `json:"port"`
	UUID                string `json:"uuid"`
	PrivateKey          string `json:"privateKey"`
	PublicKey           string `json:"publicKey"`
	ShortId             string `json:"shortId"`
	NodeIp              string `json:"nodeIp"`
	NodeHostname        string `json:"nodeHostname"`
	Status              string `json:"status"`
	Error               string `json:"error"`
	CurrentEgressIp     string `json:"currentEgressIp"`
	ContainerId         string `json:"containerId"`
	CreatedAt           int64  `json:"createdAt"`
	UpdatedAt           int64  `json:"updatedAt"`
	LastCheckTime       int64  `json:"lastCheckTime"`
	Latency             int    `json:"latency"`
	FailureCount        int    `json:"failureCount"`
	RebuildFailureCount int    `json:"rebuildFailureCount"`
}

// Job 后台任务异步Job模型结构体
type Job struct {
	Id         int64  `json:"id"`
	Type       string `json:"type"`
	Target     string `json:"target"`
	Payload    string `json:"payload"` // 保持为 JSON 字符串
	Status     string `json:"status"`
	Result     string `json:"result"` // 保持为 JSON 字符串
	Error      string `json:"error"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	StartedAt  *int64 `json:"startedAt"`
	FinishedAt *int64 `json:"finishedAt"`
}

// VPNGateNodeHistory 记录 VPNGate 节点在本系统中的历史可用性
type VPNGateNodeHistory struct {
	IP            string `json:"ip"`
	Region        string `json:"region"`
	Hostname      string `json:"hostname"`
	SuccessCount  int    `json:"successCount"`
	FailureCount  int    `json:"failureCount"`
	LastSuccessAt *int64 `json:"lastSuccessAt"`
	LastFailureAt *int64 `json:"lastFailureAt"`
	LastError     string `json:"lastError"`
	UpdatedAt     int64  `json:"updatedAt"`
}

// InitDB 初始化 SQLite 数据库，创建数据表，升级数据库字段
func InitDB(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %v", err)
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("打开数据库失败: %v", err)
	}

	// 限制最大打开连接数为 1，强制所有操作在同一个连接内串行排队，彻底避免多连接并发写入导致的 SQLITE_BUSY 锁死
	DB.SetMaxOpenConns(1)
	DB.SetMaxIdleConns(1)
	DB.SetConnMaxLifetime(0)

	// 启用 WAL 模式提高并发写入性能
	if _, err := DB.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		return fmt.Errorf("启用 WAL 模式失败: %v", err)
	}

	// 核心建表语句
	query := `
	CREATE TABLE IF NOT EXISTS egresses (
		name TEXT PRIMARY KEY,
		region TEXT NOT NULL,
		port INTEGER NOT NULL,
		uuid TEXT NOT NULL,
		privateKey TEXT NOT NULL,
		publicKey TEXT NOT NULL,
		shortId TEXT NOT NULL,
		nodeIp TEXT NOT NULL,
		nodeHostname TEXT NOT NULL,
		status TEXT NOT NULL,
		error TEXT NOT NULL DEFAULT '',
		currentEgressIp TEXT NOT NULL DEFAULT '',
		containerId TEXT NOT NULL DEFAULT '',
		createdAt INTEGER NOT NULL,
		updatedAt INTEGER NOT NULL,
		lastCheckTime INTEGER NOT NULL,
		latency INTEGER NOT NULL DEFAULT 0,
		failureCount INTEGER NOT NULL DEFAULT 0,
		rebuildFailureCount INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS jobs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		target TEXT NOT NULL DEFAULT '',
		payload TEXT NOT NULL DEFAULT '{}',
		status TEXT NOT NULL DEFAULT 'queued',
		result TEXT NOT NULL DEFAULT '{}',
		error TEXT NOT NULL DEFAULT '',
		createdAt INTEGER NOT NULL,
		updatedAt INTEGER NOT NULL,
		startedAt INTEGER,
		finishedAt INTEGER
	);

	CREATE TABLE IF NOT EXISTS vpngate_node_history (
		ip TEXT PRIMARY KEY,
		region TEXT NOT NULL,
		hostname TEXT NOT NULL DEFAULT '',
		successCount INTEGER NOT NULL DEFAULT 0,
		failureCount INTEGER NOT NULL DEFAULT 0,
		lastSuccessAt INTEGER,
		lastFailureAt INTEGER,
		lastError TEXT NOT NULL DEFAULT '',
		updatedAt INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_vpngate_node_history_region_failure
	ON vpngate_node_history(region, lastFailureAt);
	`
	if _, err := DB.Exec(query); err != nil {
		return fmt.Errorf("创建数据表失败: %v", err)
	}

	// 动态表升级：安全尝试添加 rebuildFailureCount，以防旧数据库未加载新列
	_, _ = DB.Exec("ALTER TABLE egresses ADD COLUMN rebuildFailureCount INTEGER NOT NULL DEFAULT 0")

	return nil
}

// RecordVPNGateNodeSuccess 记录 VPNGate 节点成功作为出口运行
func RecordVPNGateNodeSuccess(ip, region, hostname string) error {
	nowMs := time.Now().UnixNano() / 1e6
	_, err := DB.Exec(`
		INSERT INTO vpngate_node_history (
			ip, region, hostname, successCount, failureCount, lastSuccessAt, lastFailureAt, lastError, updatedAt
		) VALUES (?, ?, ?, 1, 0, ?, NULL, '', ?)
		ON CONFLICT(ip) DO UPDATE SET
			region = excluded.region,
			hostname = excluded.hostname,
			successCount = successCount + 1,
			lastSuccessAt = excluded.lastSuccessAt,
			lastError = '',
			updatedAt = excluded.updatedAt
	`, ip, strings.ToUpper(region), hostname, nowMs, nowMs)
	return err
}

// RecordVPNGateNodeFailure 记录 VPNGate 节点拨号或验证失败
func RecordVPNGateNodeFailure(ip, region, hostname, failure string) error {
	nowMs := time.Now().UnixNano() / 1e6
	_, err := DB.Exec(`
		INSERT INTO vpngate_node_history (
			ip, region, hostname, successCount, failureCount, lastSuccessAt, lastFailureAt, lastError, updatedAt
		) VALUES (?, ?, ?, 0, 1, NULL, ?, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET
			region = excluded.region,
			hostname = excluded.hostname,
			failureCount = failureCount + 1,
			lastFailureAt = excluded.lastFailureAt,
			lastError = excluded.lastError,
			updatedAt = excluded.updatedAt
	`, ip, strings.ToUpper(region), hostname, nowMs, failure, nowMs)
	return err
}

// GetRecentFailedVPNGateIPs 获取指定地区在窗口期内失败过的 IP
func GetRecentFailedVPNGateIPs(region string, sinceMs int64) ([]string, error) {
	rows, err := DB.Query(
		"SELECT ip FROM vpngate_node_history WHERE region = ? AND lastFailureAt IS NOT NULL AND lastFailureAt >= ?",
		strings.ToUpper(region), sinceMs,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}
	return ips, nil
}

// GetVPNGateHistoryMap 获取指定地区节点历史，用于选点综合评分
func GetVPNGateHistoryMap(region string) (map[string]VPNGateNodeHistory, error) {
	rows, err := DB.Query(`
		SELECT ip, region, hostname, successCount, failureCount, lastSuccessAt, lastFailureAt, lastError, updatedAt
		FROM vpngate_node_history WHERE region = ?
	`, strings.ToUpper(region))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]VPNGateNodeHistory)
	for rows.Next() {
		var h VPNGateNodeHistory
		if err := rows.Scan(
			&h.IP, &h.Region, &h.Hostname, &h.SuccessCount, &h.FailureCount,
			&h.LastSuccessAt, &h.LastFailureAt, &h.LastError, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result[h.IP] = h
	}
	return result, nil
}

// ListVPNGateHistory 获取指定地区最近更新的 VPNGate 节点历史记录
func ListVPNGateHistory(region string, limit int) ([]VPNGateNodeHistory, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	rows, err := DB.Query(`
		SELECT ip, region, hostname, successCount, failureCount, lastSuccessAt, lastFailureAt, lastError, updatedAt
		FROM vpngate_node_history
		WHERE region = ?
		ORDER BY updatedAt DESC
		LIMIT ?
	`, strings.ToUpper(region), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VPNGateNodeHistory
	for rows.Next() {
		var h VPNGateNodeHistory
		if err := rows.Scan(
			&h.IP, &h.Region, &h.Hostname, &h.SuccessCount, &h.FailureCount,
			&h.LastSuccessAt, &h.LastFailureAt, &h.LastError, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, nil
}

// GetEgresses 获取所有出口信息
func GetEgresses() ([]Egress, error) {
	rows, err := DB.Query("SELECT name, region, port, uuid, privateKey, publicKey, shortId, nodeIp, nodeHostname, status, error, currentEgressIp, containerId, createdAt, updatedAt, lastCheckTime, latency, failureCount, rebuildFailureCount FROM egresses ORDER BY createdAt ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Egress
	for rows.Next() {
		var e Egress
		err := rows.Scan(
			&e.Name, &e.Region, &e.Port, &e.UUID, &e.PrivateKey, &e.PublicKey, &e.ShortId,
			&e.NodeIp, &e.NodeHostname, &e.Status, &e.Error, &e.CurrentEgressIp, &e.ContainerId,
			&e.CreatedAt, &e.UpdatedAt, &e.LastCheckTime, &e.Latency, &e.FailureCount, &e.RebuildFailureCount,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, nil
}

// GetEgress 根据名称获取单个出口
func GetEgress(name string) (*Egress, error) {
	row := DB.QueryRow("SELECT name, region, port, uuid, privateKey, publicKey, shortId, nodeIp, nodeHostname, status, error, currentEgressIp, containerId, createdAt, updatedAt, lastCheckTime, latency, failureCount, rebuildFailureCount FROM egresses WHERE name = ? LIMIT 1", name)
	var e Egress
	err := row.Scan(
		&e.Name, &e.Region, &e.Port, &e.UUID, &e.PrivateKey, &e.PublicKey, &e.ShortId,
		&e.NodeIp, &e.NodeHostname, &e.Status, &e.Error, &e.CurrentEgressIp, &e.ContainerId,
		&e.CreatedAt, &e.UpdatedAt, &e.LastCheckTime, &e.Latency, &e.FailureCount, &e.RebuildFailureCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// AddEgress 插入单个新出口记录
func AddEgress(e *Egress) error {
	nowMs := time.Now().UnixNano() / 1e6
	if e.CreatedAt == 0 {
		e.CreatedAt = nowMs
	}
	e.UpdatedAt = nowMs
	e.LastCheckTime = nowMs

	_, err := DB.Exec(`INSERT INTO egresses (
		name, region, port, uuid, privateKey, publicKey, shortId,
		nodeIp, nodeHostname, status, error, currentEgressIp, containerId,
		createdAt, updatedAt, lastCheckTime, latency, failureCount, rebuildFailureCount
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Name, e.Region, e.Port, e.UUID, e.PrivateKey, e.PublicKey, e.ShortId,
		e.NodeIp, e.NodeHostname, e.Status, e.Error, e.CurrentEgressIp, e.ContainerId,
		e.CreatedAt, e.UpdatedAt, e.LastCheckTime, e.Latency, e.FailureCount, e.RebuildFailureCount,
	)
	return err
}

// UpdateEgress 动态字段安全更新出口，返回最新值
func UpdateEgress(name string, updates map[string]interface{}) (*Egress, error) {
	if len(updates) == 0 {
		return GetEgress(name)
	}

	updates["updatedAt"] = time.Now().UnixNano() / 1e6

	var clauses []string
	var args []interface{}
	for k, v := range updates {
		if k == "name" {
			continue
		}
		clauses = append(clauses, fmt.Sprintf("%s = ?", k))
		args = append(args, v)
	}

	sqlStr := fmt.Sprintf("UPDATE egresses SET %s WHERE name = ?", strings.Join(clauses, ", "))
	args = append(args, name)

	_, err := DB.Exec(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("更新 egress %s 失败: %v", name, err)
	}

	return GetEgress(name)
}

// DeleteEgress 删除单个出口记录
func DeleteEgress(name string) error {
	_, err := DB.Exec("DELETE FROM egresses WHERE name = ?", name)
	return err
}

// CountEgress 获取出口总数
func CountEgress() (int, error) {
	var count int
	err := DB.QueryRow("SELECT COUNT(1) FROM egresses").Scan(&count)
	return count, err
}

// CreateJob 创建一个新的后台任务记录
func CreateJob(jobType, target, payload string) (*Job, error) {
	nowMs := time.Now().UnixNano() / 1e6
	if payload == "" {
		payload = "{}"
	}

	res, err := DB.Exec(`INSERT INTO jobs (
		type, target, payload, status, result, error, createdAt, updatedAt, startedAt, finishedAt
	) VALUES (?, ?, ?, 'queued', '{}', '', ?, ?, NULL, NULL)`,
		jobType, target, payload, nowMs, nowMs,
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Job 失败: %v", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取 Job Insert ID 失败: %v", err)
	}

	return GetJob(id)
}

// GetJob 根据 ID 获取单个 Job 记录
func GetJob(id int64) (*Job, error) {
	row := DB.QueryRow("SELECT id, type, target, payload, status, result, error, createdAt, updatedAt, startedAt, finishedAt FROM jobs WHERE id = ? LIMIT 1", id)
	var j Job
	err := row.Scan(
		&j.Id, &j.Type, &j.Target, &j.Payload, &j.Status, &j.Result, &j.Error,
		&j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.FinishedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// UpdateJob 动态字段更新 Job
func UpdateJob(id int64, updates map[string]interface{}) (*Job, error) {
	if len(updates) == 0 {
		return GetJob(id)
	}

	updates["updatedAt"] = time.Now().UnixNano() / 1e6

	var clauses []string
	var args []interface{}
	for k, v := range updates {
		if k == "id" {
			continue
		}
		clauses = append(clauses, fmt.Sprintf("%s = ?", k))
		args = append(args, v)
	}

	sqlStr := fmt.Sprintf("UPDATE jobs SET %s WHERE id = ?", strings.Join(clauses, ", "))
	args = append(args, id)

	_, err := DB.Exec(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("更新 Job %d 失败: %v", id, err)
	}

	return GetJob(id)
}

// ListJobs 获取最近的任务列表，支持限制条数
func ListJobs(limit int) ([]Job, error) {
	rows, err := DB.Query("SELECT id, type, target, payload, status, result, error, createdAt, updatedAt, startedAt, finishedAt FROM jobs ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Job
	for rows.Next() {
		var j Job
		err := rows.Scan(
			&j.Id, &j.Type, &j.Target, &j.Payload, &j.Status, &j.Result, &j.Error,
			&j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.FinishedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, j)
	}
	return list, nil
}

// JobStatusCounts 获取各状态任务的统计数量
func JobStatusCounts() (map[string]int, error) {
	rows, err := DB.Query("SELECT status, COUNT(1) AS count FROM jobs GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, nil
}

// LatestJob 获取最新的一条任务
func LatestJob() (*Job, error) {
	row := DB.QueryRow("SELECT id, type, target, payload, status, result, error, createdAt, updatedAt, startedAt, finishedAt FROM jobs ORDER BY id DESC LIMIT 1")
	var j Job
	err := row.Scan(
		&j.Id, &j.Type, &j.Target, &j.Payload, &j.Status, &j.Result, &j.Error,
		&j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.FinishedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}
