// 负责提供 VPNGate 的 HTTP 接口控制器，支持活跃高分节点预览与地区选择列表统计
// 简体中文注释，符合中国开发者的命名与技术习惯，完美对接前端

package controllers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

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
