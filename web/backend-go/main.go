// 控制面板后端服务主入口文件，第四阶段合并 HTTP 接口路由、Basic Auth 认证、静态托管与自愈 Worker 进程
// 简体中文注释，基于 Go 1.22+ ServeMux 动态通配路由及 subtle 密码时间常数安全比对

package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vless-reality-panel/controllers"
	"vless-reality-panel/models"
	"vless-reality-panel/services"
)

var (
	startTime   = time.Now()
	versionName = "1.3.4" // 与 CHANGELOG 同步
)

// 极简 CORS 跨域辅助中间件，方便本地前端开发联调
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requirePanelAuth 时序安全的 Basic Auth 认证中间件
func requirePanelAuth(next http.Handler, username, password string, authEnabled bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 无需验证权限的公有健康检查及客户端订阅接口（因第三方客户端如 v2rayN 通常无法便利携带 Basic Auth 且避免明文泄露面板密码）
		if r.URL.Path == "/healthz" || r.URL.Path == "/api/egress/subscription" || r.URL.Path == "/api/egress/subscription.txt" {
			next.ServeHTTP(w, r)
			return
		}

		// 2. 判断是否启用账号密码鉴权
		if authEnabled {
			reqUser, reqPass, ok := r.BasicAuth()
			// 使用 crypto/subtle.ConstantTimeCompare 阻断针对 Basic Auth 的计时侧信道攻击
			userMatch := subtle.ConstantTimeCompare([]byte(reqUser), []byte(username)) == 1
			passMatch := subtle.ConstantTimeCompare([]byte(reqPass), []byte(password)) == 1

			if !ok || !userMatch || !passMatch {
				w.Header().Set("WWW-Authenticate", `Basic realm="VLESS Reality Panel", charset="UTF-8"`)
				if strings.HasPrefix(r.URL.Path, "/api") {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error":"未授权"}`))
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("Unauthorized"))
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// handleHealthz 处理 GET /healthz，非认证路由用于高可用监控
func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"service":   "vless-reality-panel",
		"version":   versionName,
		"uptime":    int(time.Since(startTime).Seconds()),
		"timestamp": time.Now().UnixNano() / 1e6,
	})
}

// handleSystemStatus 处理 GET /api/system/status，获取面板核心统计与状态
func handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	egressCount, err := models.CountEgress()
	if err != nil {
		controllers.WriteError(w, http.StatusInternalServerError, "获取出口数失败: "+err.Error())
		return
	}

	jobCounts, err := models.JobStatusCounts()
	if err != nil {
		controllers.WriteError(w, http.StatusInternalServerError, "获取任务统计失败: "+err.Error())
		return
	}

	latestJob, err := models.LatestJob()
	if err != nil {
		controllers.WriteError(w, http.StatusInternalServerError, "获取最新任务失败: "+err.Error())
		return
	}

	// 统一输出时将最新 Job 做前端字段序列化适配转换
	var compatibleJob interface{}
	if latestJob != nil {
		var p interface{}
		var res interface{}
		_ = json.Unmarshal([]byte(latestJob.Payload), &p)
		_ = json.Unmarshal([]byte(latestJob.Result), &res)

		compatibleJob = map[string]interface{}{
			"id":         latestJob.Id,
			"type":       latestJob.Type,
			"target":     latestJob.Target,
			"payload":    p,
			"status":     latestJob.Status,
			"result":     res,
			"error":      latestJob.Error,
			"createdAt":  latestJob.CreatedAt,
			"updatedAt":  latestJob.UpdatedAt,
			"startedAt":  latestJob.StartedAt,
			"finishedAt": latestJob.FinishedAt,
		}
	}

	username := os.Getenv("PANEL_USERNAME")
	password := os.Getenv("PANEL_PASSWORD")
	authEnabled := os.Getenv("PANEL_AUTH_ENABLED") != "false" && username != "" && password != ""

	controllers.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"version":         versionName,
		"authEnabled":     authEnabled,
		"egressCount":     egressCount,
		"jobStatusCounts": jobCounts,
		"latestJob":       compatibleJob,
		"uptime":          int(time.Since(startTime).Seconds()),
		"timestamp":       time.Now().UnixNano() / 1e6,
	})
}

// spaFileServer 原生单页面路由回退与静态资源混合托管处理器
func spaFileServer(distDir string) http.Handler {
	fs := http.FileServer(http.Dir(distDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API 请求如果由于漏配走到这，返回 404
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// 清理路径并计算其在该 dist 文件夹下的物理映射路径
		cleanPath := filepath.Clean(r.URL.Path)
		filePath := filepath.Join(distDir, cleanPath)

		fi, err := os.Stat(filePath)

		// 如果物理文件不存在，或者是目录（且目录下没有 index.html），则回退分发 index.html 供单页面路由解析
		if os.IsNotExist(err) || (fi != nil && fi.IsDir() && !fileExists(filepath.Join(filePath, "index.html"))) {
			http.ServeFile(w, r, filepath.Join(distDir, "index.html"))
			return
		}

		fs.ServeHTTP(w, r)
	})
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// 自动探测定位前端静态资源目录路径
func locateFrontendDist() string {
	candidates := []string{
		"./dist",
		"../frontend/dist",
		"../backend/dist",
		"/app/dist",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			absPath, _ := filepath.Abs(c)
			return absPath
		}
	}
	return ""
}

func main() {
	fmt.Printf("[*] VLESS Reality Panel Backend (Go v%s) 启动中...\n", versionName)

	// 1. 确定数据库目录路径并初始化 SQLite WAL
	dbPath := "./data/app.sqlite3"
	if _, err := os.Stat("/app/data"); err == nil {
		dbPath = "/app/data/app.sqlite3"
	}

	absDbPath, _ := filepath.Abs(dbPath)
	fmt.Printf("[*] 正在载入 SQLite 数据库: %s\n", absDbPath)

	if err := models.InitDB(dbPath); err != nil {
		fmt.Printf("[-] 数据库加载失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("[+] 数据库表结构初始化成功！WAL 写入加速已就绪")

	// 2. 并行拉起后台异步任务队列 WorkerPool 及测活自愈心跳
	services.StartWorkerPool()
	services.StartSelfHealing()
	fmt.Println("[+] 后台异步队列消费协程与出口自愈守护已成功拉起")

	// 3. 构建原生多路复用 API 路由器 (依赖 Go 1.22+ 动态路径匹配)
	mux := http.NewServeMux()

	// 免认证公有健康检查
	mux.HandleFunc("GET /healthz", handleHealthz)

	// 系统信息与统计
	mux.HandleFunc("GET /api/system/status", handleSystemStatus)

	// 出口生命周期接口
	mux.HandleFunc("POST /api/egress/create", controllers.CreateEgressHandler)
	mux.HandleFunc("POST /api/egress/create-all", controllers.CreateAllEgressHandler)
	mux.HandleFunc("GET /api/egress/list", controllers.ListEgressHandler)
	mux.HandleFunc("POST /api/egress/delete", controllers.DeleteEgressHandler)
	mux.HandleFunc("POST /api/egress/delete-all", controllers.DeleteAllEgressHandler)
	mux.HandleFunc("POST /api/egress/rebuild", controllers.RebuildEgressHandler)
	mux.HandleFunc("POST /api/egress/rebuild-all", controllers.RebuildAllEgressHandler)
	mux.HandleFunc("GET /api/egress/subscription", controllers.GetSubscriptionHandler)
	mux.HandleFunc("GET /api/egress/subscription.txt", controllers.GetSubscriptionTextHandler)
	mux.HandleFunc("GET /api/egress/jobs", controllers.ListJobsHandler)
	mux.HandleFunc("GET /api/egress/{name}/link", controllers.GetEgressLinkHandler)
	mux.HandleFunc("GET /api/egress/{name}/logs", controllers.GetEgressLogsHandler)

	// VPNGate 镜像资源数据查询接口
	mux.HandleFunc("GET /api/vpngate/nodes", controllers.GetNodes)
	mux.HandleFunc("GET /api/vpngate/regions", controllers.GetRegions)
	mux.HandleFunc("GET /api/vpngate/candidates", controllers.GetCandidates)
	mux.HandleFunc("GET /api/vpngate/history", controllers.GetHistory)

	// 4. 前端静态包服务挂载与 SPA 自愈回退
	distDir := locateFrontendDist()
	if distDir != "" {
		fmt.Printf("[+] 成功探测到前端静态资源包 dist 路径: %s\n", distDir)
		mux.Handle("/", spaFileServer(distDir))
	} else {
		// 前端 dist 资源未打包时的友好报错引导
		fmt.Println("[!] 警告: 未能在当前工作区找到已编译的前端 dist，将启用 fallback 引导页")
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<h1 style="text-align:center;margin-top:100px;font-family:sans-serif;">VLESS Reality Egress Panel Backend (Go) is running.<br>前端页面尚未打包编译，请进入 web/frontend 执行 npm run build 编译。</h1>`))
		})
	}

	// 5. 绑定 Basic Auth 时序安全中间件及极简 CORS 控制
	username := os.Getenv("PANEL_USERNAME")
	password := os.Getenv("PANEL_PASSWORD")
	authEnabled := os.Getenv("PANEL_AUTH_ENABLED") != "false" && username != "" && password != ""

	if authEnabled {
		fmt.Printf("[+] 安全机制：控制面板 Basic Auth 验证已成功挂载 (管理员账号: %s)\n", username)
	} else {
		fmt.Println("[!] 安全提示：未配置管理员凭证或 PANEL_AUTH_ENABLED 被停用，接口将匿名放行")
	}

	authHandler := requirePanelAuth(mux, username, password, authEnabled)
	finalHandler := corsMiddleware(authHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	fmt.Printf("[+] 服务端已成功绑定端口 %s，正在等待网络连接...\n", port)
	if err := http.ListenAndServe(":"+port, finalHandler); err != nil {
		fmt.Printf("[-] HTTP 服务端异常退出: %v\n", err)
		os.Exit(1)
	}
}
