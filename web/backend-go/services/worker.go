// 负责后台任务（Job）的高性能消费逻辑，支持任务队列持久化恢复与双 Worker 协程消费
// 简体中文注释，基于 sync.Map 实现具有互斥锁性质的 Target 保护，防止同出口容器产生 Docker 并发竞态

package services

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"vless-reality-panel/models"
)

// JobChan 后台任务缓冲通道队列
var JobChan = make(chan int64, 100)

// activeTargets 用于在运行中锁定同一出口
var activeTargets sync.Map

// acquireTargetLock 尝试锁住特定 Target 任务
func acquireTargetLock(target string) bool {
	if target == "" {
		return true // 无 Target，例如批量删除/重建任务，无需加锁
	}
	_, loaded := activeTargets.LoadOrStore(target, true)
	return !loaded // 如果不存在则成功存入并锁定（返回 true），若存在说明已被其他 Worker 占用（返回 false）
}

// releaseTargetLock 释放特定 Target 锁
func releaseTargetLock(target string) {
	if target == "" {
		return
	}
	activeTargets.Delete(target)
}

// StartWorkerPool 启动双并发 Worker 协程，并自动从 SQLite 恢复未处理的 queued 任务
func StartWorkerPool() {
	// 启动 2 个 Worker 并发消费任务
	for i := 1; i <= 2; i++ {
		go func(workerID int) {
			fmt.Printf("[*] 异步任务 Worker-%d 已启动，开始监听任务队列...\n", workerID)
			for jobID := range JobChan {
				processJob(jobID)
			}
		}(i)
	}

	// 启动协程安全加载数据库挂起（queued）的任务并载入通道
	go func() {
		// 延时等待数据库初始化完成
		time.Sleep(500 * time.Millisecond)

		rows, err := models.DB.Query("SELECT id FROM jobs WHERE status = 'queued' ORDER BY id ASC")
		if err != nil {
			fmt.Printf("[!] 从数据库载入排队任务失败: %v\n", err)
			return
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err == nil {
				JobChan <- id
				count++
			}
		}
		if count > 0 {
			fmt.Printf("[+] 成功从 SQLite 数据库中自动加载并恢复了 %d 个排队任务到队列\n", count)
		}
	}()
}

// SubmitJob 将任务 ID 塞入队列
func SubmitJob(id int64) {
	select {
	case JobChan <- id:
	default:
		fmt.Printf("[!] 任务队列已满，未能即时投递任务 ID: %d\n", id)
	}
}

// processJob 核心处理单条任务
func processJob(jobID int64) {
	job, err := models.GetJob(jobID)
	if err != nil {
		fmt.Printf("[!] 获取任务 ID %d 记录出错: %v\n", jobID, err)
		return
	}
	if job == nil {
		return
	}

	// 双重验证状态，防止重复执行
	if job.Status != "queued" {
		return
	}

	// 1. 尝试获取该出口的 Target 互斥锁
	if !acquireTargetLock(job.Target) {
		// 锁定失败（正在执行同 target 任务），则延迟 1 秒放回通道末尾，并退出本次读取
		time.AfterFunc(1*time.Second, func() {
			JobChan <- jobID
		})
		return
	}
	defer releaseTargetLock(job.Target)

	// 2. 状态设置为运行中
	nowMs := time.Now().UnixNano() / 1e6
	_, err = models.UpdateJob(jobID, map[string]interface{}{
		"status":    "running",
		"startedAt": nowMs,
		"error":     "",
	})
	if err != nil {
		fmt.Printf("[!] 更新任务 ID %d 状态为 running 失败: %v\n", jobID, err)
		return
	}

	fmt.Printf("[*] 正在执行后台任务 [ID: %d, 类型: %s, 目标: %s]...\n", job.Id, job.Type, job.Target)

	var result interface{}
	var runErr error

	// 3. 根据任务类型解析 Payload 并调用对应的业务接口
	switch job.Type {
	case "create":
		var p struct {
			Name   string `json:"name"`
			Region string `json:"region"`
			UUID   string `json:"uuid"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
			runErr = fmt.Errorf("反序列化 Payload 失败: %v", err)
		} else {
			result, runErr = CreateEgress(p.Name, p.Region, p.UUID)
		}

	case "create-all":
		var p struct {
			Regions []string `json:"regions"`
			UUID    string   `json:"uuid"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
			runErr = fmt.Errorf("反序列化 Payload 失败: %v", err)
		} else {
			result, runErr = CreateAll(p.Regions, p.UUID)
		}

	case "delete":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
			runErr = fmt.Errorf("反序列化 Payload 失败: %v", err)
		} else {
			runErr = DeleteEgress(p.Name)
			result = map[string]interface{}{"name": p.Name}
		}

	case "delete-all":
		res, err := DeleteAll()
		result = map[string]interface{}{"deleted": res}
		runErr = err

	case "rebuild":
		var p struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
			runErr = fmt.Errorf("反序列化 Payload 失败: %v", err)
		} else {
			result, runErr = RebuildEgress(p.Name)
		}

	case "rebuild-all":
		res, err := RebuildAll()
		result = map[string]interface{}{"results": res}
		runErr = err

	default:
		runErr = fmt.Errorf("未知的任务类型: %s", job.Type)
	}

	finishedMs := time.Now().UnixNano() / 1e6

	// 4. 将执行结果回写数据库，完成生命周期
	if runErr != nil {
		fmt.Printf("[!] 后台任务执行失败 [ID: %d, 类型: %s]: %v\n", job.Id, job.Type, runErr)
		_, _ = models.UpdateJob(jobID, map[string]interface{}{
			"status":     "failed",
			"error":      runErr.Error(),
			"finishedAt": finishedMs,
			"result":     "{}",
		})
	} else {
		fmt.Printf("[+] 后台任务执行成功 [ID: %d, 类型: %s]\n", job.Id, job.Type)
		resBytes, _ := json.Marshal(result)
		_, _ = models.UpdateJob(jobID, map[string]interface{}{
			"status":     "done",
			"error":      "",
			"finishedAt": finishedMs,
			"result":     string(resBytes),
		})
	}
}
