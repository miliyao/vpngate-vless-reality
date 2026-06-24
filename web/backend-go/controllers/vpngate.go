// 负责提供 VPNGate 的 HTTP 接口控制器，支持活跃高分节点预览与地区选择列表统计
// 简体中文注释，符合中国开发者的命名与技术习惯，完美对接前端

package controllers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"vless-reality-panel/models"
	"vless-reality-panel/services"
)

// NodePreview 前端展示的节点预览结构体
type NodePreview struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Ping     int    `json:"ping"`
	Speed    int64  `json:"speed"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	Score    int64  `json:"score"`
	Sessions int    `json:"sessions"`
}

// RegionCount 前端下拉菜单国家/地区及节点数统计结构体
type RegionCount struct {
	Code  string `json:"code"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// CandidatePreview 展示 VPNGate 候选节点评分，不暴露完整 OpenVPN 配置
type CandidatePreview struct {
	Hostname       string  `json:"hostname"`
	IP             string  `json:"ip"`
	Region         string  `json:"region"`
	Ping           int     `json:"ping"`
	Speed          int64   `json:"speed"`
	Score          int64   `json:"score"`
	Sessions       int     `json:"sessions"`
	Uptime         int64   `json:"uptime"`
	Proto          string  `json:"proto"`
	RemoteHost     string  `json:"remoteHost"`
	RemotePort     string  `json:"remotePort"`
	SelectionScore float64 `json:"selectionScore"`
}

// WriteJSON 辅助函数：输出 JSON 响应数据
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// WriteError 辅助函数：输出规范化的 JSON 错误信息
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

func queryLimit(r *http.Request, defaultLimit, maxLimit int) int {
	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if val, err := strconv.Atoi(raw); err == nil {
			limit = val
		}
	}
	if limit < 1 {
		return 1
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func queryRegion(r *http.Request) (string, bool) {
	region := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("region")))
	if len(region) != 2 {
		return "", false
	}
	return region, true
}

// GetNodes 处理 GET /api/vpngate/nodes 请求，返回前 30 个高分活跃节点
func GetNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := services.FetchNodes()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "获取 VPNGate 节点失败: "+err.Error())
		return
	}

	previewNodes := make([]NodePreview, 0)
	for _, n := range nodes {
		if n.IP == "" || n.CountryShort == "" {
			continue
		}
		previewNodes = append(previewNodes, NodePreview{
			Hostname: n.Hostname,
			IP:       n.IP,
			Ping:     n.Ping,
			Speed:    n.Speed,
			Country:  n.CountryLong,
			Region:   n.CountryShort,
			Score:    n.Score,
			Sessions: n.NumSessions,
		})
	}

	// 截取前 30 个（FetchNodes 已经经过内部基于 Score 排序，这里直接提取前 30 个即可）
	if len(previewNodes) > 30 {
		previewNodes = previewNodes[:30]
	}

	WriteJSON(w, http.StatusOK, previewNodes)
}

// GetRegions 处理 GET /api/vpngate/regions 请求，返回可选的国家简码及其活跃节点统计数
func GetRegions(w http.ResponseWriter, r *http.Request) {
	nodes, err := services.FetchNodes()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "获取地区列表失败: "+err.Error())
		return
	}

	regionMap := make(map[string]*RegionCount)
	for _, n := range nodes {
		code := strings.ToUpper(n.CountryShort)
		name := n.CountryLong
		if code == "" {
			continue
		}
		if _, ok := regionMap[code]; !ok {
			regionMap[code] = &RegionCount{
				Code:  code,
				Name:  name,
				Count: 0,
			}
		}
		regionMap[code].Count++
	}

	list := make([]RegionCount, 0)
	for _, v := range regionMap {
		list = append(list, *v)
	}

	// 按节点活跃数降序排序
	sort.Slice(list, func(i, j int) bool {
		return list[i].Count > list[j].Count
	})

	WriteJSON(w, http.StatusOK, list)
}

// GetCandidates 处理 GET /api/vpngate/candidates?region=JP，返回当前选点候选评分
func GetCandidates(w http.ResponseWriter, r *http.Request) {
	region, ok := queryRegion(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "region 参数必须是两位国家/地区代码，例如 JP")
		return
	}

	limit := queryLimit(r, 8, 50)
	candidates, err := services.GetCandidateNodes(region, nil, limit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "获取 VPNGate 候选节点失败: "+err.Error())
		return
	}

	result := make([]CandidatePreview, 0, len(candidates))
	for _, n := range candidates {
		result = append(result, CandidatePreview{
			Hostname:       n.Hostname,
			IP:             n.IP,
			Region:         n.Country,
			Ping:           n.Ping,
			Speed:          n.Speed,
			Score:          n.Score,
			Sessions:       n.Sessions,
			Uptime:         n.Uptime,
			Proto:          n.Proto,
			RemoteHost:     n.RemoteHost,
			RemotePort:     n.RemotePort,
			SelectionScore: n.SelectionScore,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"region":     region,
		"limit":      limit,
		"candidates": result,
	})
}

// GetHistory 处理 GET /api/vpngate/history?region=JP，返回本系统记录的节点成功/失败历史
func GetHistory(w http.ResponseWriter, r *http.Request) {
	region, ok := queryRegion(r)
	if !ok {
		WriteError(w, http.StatusBadRequest, "region 参数必须是两位国家/地区代码，例如 JP")
		return
	}

	limit := queryLimit(r, 50, 200)
	history, err := models.ListVPNGateHistory(region, limit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "获取 VPNGate 节点历史失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"region":  region,
		"limit":   limit,
		"history": history,
	})
}
