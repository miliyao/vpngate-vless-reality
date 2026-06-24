// 负责提供出口代理、订阅及任务查询的 HTTP 接口控制器
// 简体中文注释，符合中国开发者的命名与技术习惯，对 Job 返回做防二次序列化纯 JSON 解析处理

package controllers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"vless-reality-panel/models"
	"vless-reality-panel/services"

	"github.com/google/uuid"
)

var egressNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// JobJSONResponse 兼容前端的 Job 返回格式包装
type JobJSONResponse struct {
	Id         int64       `json:"id"`
	Type       string      `json:"type"`
	Target     string      `json:"target"`
	Payload    interface{} `json:"payload"` // 解析为 JSON 节点而非转义字符串
	Status     string      `json:"status"`
	Result     interface{} `json:"result"` // 解析为 JSON 节点而非转义字符串
	Error      string      `json:"error"`
	CreatedAt  int64       `json:"createdAt"`
	UpdatedAt  int64       `json:"updatedAt"`
	StartedAt  *int64      `json:"startedAt"`
	FinishedAt *int64      `json:"finishedAt"`
}

// validateEgressName 校验出口名称格式
func validateEgressName(name string) bool {
	return name != "" && egressNameRegexp.MatchString(name)
}

// getVpsHost 获取 VPS 出网的主机地址
func getVpsHost(r *http.Request) string {
	if addr := os.Getenv("VPS_ADDRESS"); addr != "" {
		return addr
	}
	if host := r.Header.Get("Host"); host != "" {
		hostOnly, _, err := net.SplitHostPort(host)
		if err != nil {
			return host // 如果没有端口直接返回
		}
		return hostOnly
	}
	return "your_vps_ip"
}

// buildSubscriptionPayload 拼装客户端订阅 Base64 和链接数据
func buildSubscriptionPayload(r *http.Request) (string, []string, int, error) {
	list, err := models.GetEgresses()
	if err != nil {
		return "", nil, 0, err
	}

	vpsHost := getVpsHost(r)
	links := make([]string, 0)
	for _, e := range list {
		links = append(links, services.BuildLink(&e, vpsHost))
	}

	rawSub := strings.Join(links, "\n")
	subscription := base64.StdEncoding.EncodeToString([]byte(rawSub))

	return subscription, links, len(links), nil
}

// enqueueJob 封装任务创建与派发的通用操作
func enqueueJob(jobType, target string, payload interface{}) (*models.Job, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化 Payload 失败: %v", err)
	}

	job, err := models.CreateJob(jobType, target, string(payloadBytes))
	if err != nil {
		return nil, err
	}

	// 提报给异步消费通道调度执行
	services.SubmitJob(job.Id)
	return job, nil
}

// toJobJSONResponse 将 models.Job 转换为 JSON 兼容结构
func toJobJSONResponse(j *models.Job) JobJSONResponse {
	var p interface{}
	var r interface{}

	if err := json.Unmarshal([]byte(j.Payload), &p); err != nil {
		p = map[string]interface{}{}
	}
	if err := json.Unmarshal([]byte(j.Result), &r); err != nil {
		r = map[string]interface{}{}
	}

	return JobJSONResponse{
		Id:         j.Id,
		Type:       j.Type,
		Target:     j.Target,
		Payload:    p,
		Status:     j.Status,
		Result:     r,
		Error:      j.Error,
		CreatedAt:  j.CreatedAt,
		UpdatedAt:  j.UpdatedAt,
		StartedAt:  j.StartedAt,
		FinishedAt: j.FinishedAt,
	}
}

// CreateEgressHandler 处理 POST /api/egress/create，提交创建出口任务
func CreateEgressHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name   string `json:"name"`
		Region string `json:"region"`
		UUID   string `json:"uuid"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "无效的 JSON 请求体")
		return
	}

	if !validateEgressName(body.Name) {
		WriteError(w, http.StatusBadRequest, "出口名称无效，仅允许英文字母、数字和横杠下划线")
		return
	}
	if len(body.Region) != 2 {
		WriteError(w, http.StatusBadRequest, "地区代码无效，必须为两位大写字母简写")
		return
	}
	if body.UUID != "" {
		if _, err := uuid.Parse(body.UUID); err != nil {
			WriteError(w, http.StatusBadRequest, "UUID 格式无效")
			return
		}
	} else {
		body.UUID = uuid.New().String()
	}

	job, err := enqueueJob("create", body.Name, body)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "提交任务失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"jobId":   job.Id,
		"message": fmt.Sprintf("出口 %s 创建任务已提交", body.Name),
	})
}

// CreateAllEgressHandler 处理 POST /api/egress/create-all，提交批量创建出口任务
func CreateAllEgressHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Regions []string `json:"regions"`
		UUID    string   `json:"uuid"`
	}

	// 允许 Body 为空，默认创建所有可用
	_ = json.NewDecoder(r.Body).Decode(&body)

	if body.UUID != "" {
		if _, err := uuid.Parse(body.UUID); err != nil {
			WriteError(w, http.StatusBadRequest, "UUID 格式无效")
			return
		}
	} else {
		body.UUID = uuid.New().String()
	}

	job, err := enqueueJob("create-all", "all", body)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "提交任务失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"jobId":   job.Id,
		"message": "批量创建任务已提交",
	})
}

// ListEgressHandler 处理 GET /api/egress/list，并发拉取 Docker 并执行增量数据比对同步
func ListEgressHandler(w http.ResponseWriter, r *http.Request) {
	list, err := models.GetEgresses()
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "查询出口列表失败: "+err.Error())
		return
	}

	type checkResult struct {
		report *services.ContainerStatusReport
		err    error
	}

	settled := make([]checkResult, len(list))
	var wg sync.WaitGroup

	// 并发触发 Docker 测活
	for i, e := range list {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			report, err := services.GetContainerStatusAndIp(name)
			settled[idx] = checkResult{report: report, err: err}
		}(i, e.Name)
	}

	wg.Wait()

	nowMs := time.Now().UnixNano() / 1e6
	results := make([]models.Egress, 0)

	for i, egress := range list {
		res := settled[i]
		var dockerStatus *services.ContainerStatusReport

		if res.err != nil {
			dockerStatus = &services.ContainerStatusReport{
				Status: "error",
				IP:     "error",
				Error:  "Docker 查询失败: " + res.err.Error(),
			}
		} else {
			dockerStatus = res.report
		}

		// 若在启动或重建中而 Docker 暂时 offline，则继续维持数据库中原来的 starting/rebuilding 状态而非直接变成 error
		shouldKeep := (egress.Status == "starting" || egress.Status == "rebuilding") && dockerStatus.Status == "offline"
		
		resolvedStatus := dockerStatus.Status
		if dockerStatus.IP == "error" {
			resolvedStatus = "error"
		}

		updates := map[string]interface{}{
			"error":         dockerStatus.Error,
			"lastCheckTime": nowMs,
		}

		if shouldKeep {
			updates["status"] = egress.Status
		} else {
			updates["status"] = resolvedStatus
		}

		if dockerStatus.IP != "" && dockerStatus.IP != "offline" && dockerStatus.IP != "error" {
			updates["currentEgressIp"] = dockerStatus.IP
		}

		// 增量比较：仅当字段存在实质性变更，方才写入 SQLite 以最大化减少落盘损耗
		statusChanged := false
		if val, ok := updates["status"].(string); ok && val != egress.Status {
			statusChanged = true
		}
		errorChanged := false
		if val, ok := updates["error"].(string); ok && val != egress.Error {
			errorChanged = true
		}
		ipChanged := false
		if val, ok := updates["currentEgressIp"].(string); ok && val != egress.CurrentEgressIp {
			ipChanged = true
		}

		if statusChanged || errorChanged || ipChanged {
			updated, err := models.UpdateEgress(egress.Name, updates)
			if err == nil && updated != nil {
				results = append(results, *updated)
				continue
			}
		}

		// 无变化则在内存中合并字段直接返回，省去磁盘 I/O 开销
		merged := egress
		merged.LastCheckTime = nowMs
		if val, ok := updates["status"].(string); ok {
			merged.Status = val
		}
		if val, ok := updates["error"].(string); ok {
			merged.Error = val
		}
		if val, ok := updates["currentEgressIp"].(string); ok {
			merged.CurrentEgressIp = val
		}
		results = append(results, merged)
	}

	WriteJSON(w, http.StatusOK, results)
}

// DeleteEgressHandler 处理 POST /api/egress/delete，提交销毁单个出口任务
func DeleteEgressHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "无效的 JSON 请求体")
		return
	}

	if !validateEgressName(body.Name) {
		WriteError(w, http.StatusBadRequest, "出口名称格式错误")
		return
	}

	job, err := enqueueJob("delete", body.Name, body)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "提交删除任务失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"jobId":   job.Id,
		"message": fmt.Sprintf("出口 %s 删除任务已提交", body.Name),
	})
}

// DeleteAllEgressHandler 处理 POST /api/egress/delete-all，提交删除所有出口任务
func DeleteAllEgressHandler(w http.ResponseWriter, r *http.Request) {
	job, err := enqueueJob("delete-all", "all", map[string]interface{}{})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "提交任务失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"jobId":   job.Id,
		"message": "删除全部任务已提交",
	})
}

// RebuildEgressHandler 处理 POST /api/egress/rebuild，提交漂移重建单个出口任务
func RebuildEgressHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "无效的 JSON 请求体")
		return
	}

	if !validateEgressName(body.Name) {
		WriteError(w, http.StatusBadRequest, "出口名称格式错误")
		return
	}

	job, err := enqueueJob("rebuild", body.Name, body)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "提交漂移任务失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"jobId":   job.Id,
		"message": fmt.Sprintf("出口 %s 漂移任务已提交", body.Name),
	})
}

// RebuildAllEgressHandler 处理 POST /api/egress/rebuild-all，提交漂移所有出口任务
func RebuildAllEgressHandler(w http.ResponseWriter, r *http.Request) {
	job, err := enqueueJob("rebuild-all", "all", map[string]interface{}{})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "提交任务失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"jobId":   job.Id,
		"message": "批量漂移任务已提交",
	})
}

// GetSubscriptionHandler 处理 GET /api/egress/subscription，输出 VLESS 订阅 JSON 对象
func GetSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	subscription, links, count, err := buildSubscriptionPayload(r)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "生成订阅失败: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"subscription": subscription,
		"links":        links,
		"count":        count,
	})
}

// GetSubscriptionTextHandler 处理 GET /api/egress/subscription.txt，输出 Base64 的文本
func GetSubscriptionTextHandler(w http.ResponseWriter, r *http.Request) {
	subscription, _, _, err := buildSubscriptionPayload(r)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("生成订阅失败: " + err.Error()))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(subscription))
}

// ListJobsHandler 处理 GET /api/egress/jobs，拉取最近的任务历史
func ListJobsHandler(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if val, err := strconv.Atoi(limitStr); err == nil {
			limit = val
		}
	}

	// 限制数值范围在 1 到 200 之间，防大量数据拖垮系统
	if limit < 1 {
		limit = 1
	} else if limit > 200 {
		limit = 200
	}

	rawJobs, err := models.ListJobs(limit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "获取任务历史失败: "+err.Error())
		return
	}

	// 将字符串 payload/result 解构为原始 JSON 结构再进行输出，兼容前端
	results := make([]JobJSONResponse, 0)
	for _, j := range rawJobs {
		results = append(results, toJobJSONResponse(&j))
	}

	WriteJSON(w, http.StatusOK, results)
}

// GetEgressLinkHandler 处理 GET /api/egress/{name}/link，返回单个出口的 VLESS Reality 单路由订阅链接
func GetEgressLinkHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name") // 依赖 Go 1.22+ 原生通配占位符提取
	if !validateEgressName(name) {
		WriteError(w, http.StatusBadRequest, "出口名称格式错误")
		return
	}

	egress, err := models.GetEgress(name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "获取出口信息失败: "+err.Error())
		return
	}
	if egress == nil {
		WriteError(w, http.StatusNotFound, "未找到该出口")
		return
	}

	link := services.BuildLink(egress, getVpsHost(r))
	WriteJSON(w, http.StatusOK, map[string]string{"link": link})
}

// GetEgressLogsHandler 处理 GET /api/egress/{name}/logs，拉取出口容器的 OpenVPN / Xray 系统日志
func GetEgressLogsHandler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !validateEgressName(name) {
		WriteError(w, http.StatusBadRequest, "出口名称格式错误")
		return
	}

	egress, err := models.GetEgress(name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "检查出口失败: "+err.Error())
		return
	}
	if egress == nil {
		WriteError(w, http.StatusNotFound, "该出口未被创建")
		return
	}

	tailStr := r.URL.Query().Get("tail")
	tail := 120
	if tailStr != "" {
		if val, err := strconv.Atoi(tailStr); err == nil {
			tail = val
		}
	}

	// 合理化 tail 限制
	if tail < 20 {
		tail = 20
	} else if tail > 500 {
		tail = 500
	}

	logs, err := services.GetContainerLogs(name, tail)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "读取日志流异常: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"logs": logs})
}
