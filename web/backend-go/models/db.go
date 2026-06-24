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

// InitDB 初始化 SQLite 数据库，创建数据表，升级数据库字段
func InitDB(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建数据目录失败: %v", err)
	}

	var err error
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %v", err)
	}

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
	`
	if _, err := DB.Exec(query); err != nil {
		return fmt.Errorf("创建数据表失败: %v", err)
	}

	// 动态表升级：安全尝试添加 rebuildFailureCount，以防旧数据库未加载新列
	_, _ = DB.Exec("ALTER TABLE egresses ADD COLUMN rebuildFailureCount INTEGER NOT NULL DEFAULT 0")

	return nil
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
