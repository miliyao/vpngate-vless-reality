// 负责出口自愈测活守护，定期心跳测活，一旦出口失效自动创建重建 Job 并派发
// 简体中文注释，基于 Go time.NewTicker 实现平滑心跳，并在协程池中并发检测

package services

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"vless-reality-panel/models"
)

var (
	ticker     *time.Ticker
	stopTicker chan struct{}
	tickerLock sync.Mutex
)

// getEnvInt 辅助获取整数环境变量
func getEnvInt(key string, defaultVal int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultVal
	}
	return val
}

// StartSelfHealing 启动测活自愈心跳守护服务
func StartSelfHealing() {
	tickerLock.Lock()
	defer tickerLock.Unlock()

	if ticker != nil {
		return
	}

	// 默认 24 小时检测一次，可通过环境变量进行调优
	intervalMs := getEnvInt("HEALTH_CHECK_INTERVAL", 86400000)
	interval := time.Duration(intervalMs) * time.Millisecond

	ticker = time.NewTicker(interval)
	stopTicker = make(chan struct{})

	fmt.Printf("[*] 自愈守护服务已启动，测活心跳间隔: %v\n", interval)

	go func() {
		for {
			select {
			case <-ticker.C:
				fmt.Println("[*] 开始执行定时自愈测活检查...")
				performHealthCheck()
			case <-stopTicker:
				return
			}
		}
	}()
}

// StopSelfHealing 停止测活自愈服务
func StopSelfHealing() {
	tickerLock.Lock()
	defer tickerLock.Unlock()

	if ticker == nil {
		return
	}

	ticker.Stop()
	close(stopTicker)
	ticker = nil
	fmt.Println("[*] 自愈守护服务已停止")
}

// performHealthCheck 执行各活跃出口的并发测活逻辑
func performHealthCheck() {
	egresses, err := models.GetEgresses()
	if err != nil {
		fmt.Printf("[!] 自愈检测：获取出口列表失败: %v\n", err)
		return
	}

	// 筛选出可以被检测的出口（排除正处于启动或重建状态的）
	var targets []models.Egress
	for _, e := range egresses {
		if e.Status != "starting" && e.Status != "rebuilding" {
			targets = append(targets, e)
		}
	}

	if len(targets) == 0 {
		return
	}

	type checkResult struct {
		egress models.Egress
		report *ContainerStatusReport
		err    error
	}

	results := make([]checkResult, len(targets))
	var wg sync.WaitGroup

	// 并发对所有出口向 Docker SDK 触发 IP 和连通性测活
	for i, target := range targets {
		wg.Add(1)
		go func(idx int, eg models.Egress) {
			defer wg.Done()
			report, err := GetContainerStatusAndIp(eg.Name)
			results[idx] = checkResult{
				egress: eg,
				report: report,
				err:    err,
			}
		}(i, target)
	}

	wg.Wait()

	nowMs := time.Now().UnixNano() / 1e6
	maxFailures := getEnvInt("MAX_FAILURES", 2)

	for _, res := range results {
		eg := res.egress
		name := eg.Name
		var statusReport *ContainerStatusReport

		if res.err != nil {
			statusReport = &ContainerStatusReport{
				Status: "error",
				IP:     "error",
				Error:  fmt.Sprintf("测活异常: %v", res.err),
			}
		} else {
			statusReport = res.report
		}

		// 如果状态不为 running 或无法获取外部 IP，则认为出口断开连通
		if statusReport.Status != "running" || statusReport.IP == "error" {
			newCount := eg.FailureCount + 1
			_, _ = models.UpdateEgress(name, map[string]interface{}{
				"failureCount":  newCount,
				"error":         statusReport.Error,
				"lastCheckTime": nowMs,
			})
			fmt.Printf("[!] 出口 %s 测活断开 (%d/%d): %s\n", name, newCount, maxFailures, statusReport.Error)

			// 故障数达到熔断上限，触发自愈
			if newCount >= maxFailures {
				fmt.Printf("[!] 触发自愈：出口 %s 连续故障达上限，开始提交漂移重建任务...\n", name)

				// 立即标记为 rebuild 状态，防止被下一次心跳检测重复扫到
				_, err := models.UpdateEgress(name, map[string]interface{}{
					"status":       "rebuilding",
					"failureCount": 0,
					"error":        "",
				})
				if err != nil {
					fmt.Printf("[!] 自愈标记失败 %s: %v\n", name, err)
					continue
				}

				// 将耗时的重建操作生成 Job 存入 SQLite 任务队列，实现异步解耦消费
				payloadBytes, _ := json.Marshal(map[string]string{"name": name})
				job, err := models.CreateJob("rebuild", name, string(payloadBytes))
				if err != nil {
					_, _ = models.UpdateEgress(name, map[string]interface{}{
						"status": "error",
						"error":  fmt.Sprintf("自愈入队失败: %v", err),
					})
					fmt.Printf("[-] [自愈入队失败] %s: %v\n", name, err)
				} else {
					// 塞入内存通道触发立即调度
					SubmitJob(job.Id)
					fmt.Printf("[+] [自愈已委派] 出口 %s 重建 Job-%d 已推入任务队列\n", name, job.Id)
				}
			}
		} else {
			// 检测通过，重置 failureCount 故障计数
			_, _ = models.UpdateEgress(name, map[string]interface{}{
				"failureCount":  0,
				"error":         "",
				"lastCheckTime": nowMs,
			})
		}
	}
}
