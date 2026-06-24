// 封装 Docker SDK，负责管理出口 Egress 容器的生命周期与测活
// 简体中文注释，基于官方 Docker SDK 实现，支持容器快速重启复用与 demux 拆流解复用

package services

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"vless-reality-panel/models"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

const (
	egressImage     = "vpngate-egress:latest"
	cacheMaxAge     = 6 * time.Second
	containerPrefix = "egress-"
)

// ContainerStatusReport 容器测活状态报告
type ContainerStatusReport struct {
	Status string `json:"status"`
	IP     string `json:"ip"`
	Error  string `json:"error"`
}

var (
	dockerCli           *client.Client
	cachedHostDataPath  string
	hostPathOnce        sync.Once
	statusCache         = make(map[string]cachedReport)
	statusCacheMutex    sync.RWMutex
	ipv4Regexp          = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
	initDockerClientOnce sync.Once
	dockerInitErr       error
)

type cachedReport struct {
	timestamp time.Time
	data      *ContainerStatusReport
}

// 获取/初始化 Docker Client 单例
func getDockerClient() (*client.Client, error) {
	initDockerClientOnce.Do(func() {
		dockerCli, dockerInitErr = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if dockerInitErr != nil {
			dockerInitErr = fmt.Errorf("初始化 Docker 客户端失败: %v", dockerInitErr)
		}
	})
	return dockerCli, dockerInitErr
}

// resolveHostDataPath 自动推导宿主机挂载的真实数据目录
func resolveHostDataPath() string {
	hostPathOnce.Do(func() {
		// 方式一：环境变量指定
		if envPath := os.Getenv("HOST_DATA_PATH"); envPath != "" {
			cachedHostDataPath = envPath
			fmt.Printf("[*] 从环境变量获取宿主机 Data 路径: %s\n", cachedHostDataPath)
			return
		}

		// 方式二：Inspect 面板自身容器
		cli, err := getDockerClient()
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if info, err := cli.ContainerInspect(ctx, "vless-web-panel"); err == nil {
				for _, m := range info.Mounts {
					if m.Destination == "/app/data" {
						cachedHostDataPath = m.Source
						fmt.Printf("[*] 自动探测到宿主机 Data 真实路径: %s\n", cachedHostDataPath)
						return
					}
				}
			}
		}

		// 方式三：默认兜底
		cachedHostDataPath = "/app/data"
		fmt.Printf("[!] 未能推导宿主机路径，使用默认值: %s\n", cachedHostDataPath)
	})
	return cachedHostDataPath
}

// StartEgressContainer 启动或创建出口容器，若存在则快速重启复用
func StartEgressContainer(egress *models.Egress) (string, error) {
	cli, err := getDockerClient()
	if err != nil {
		return "", err
	}

	containerName := containerPrefix + egress.Name
	hostDataPath := resolveHostDataPath()
	hostEgressDir := fmt.Sprintf("%s/egress/%s", hostDataPath, egress.Name)

	ctx := context.Background()

	// 1. 检查同名容器是否已存在，如果存在则直接重启复用
	info, err := cli.ContainerInspect(ctx, containerName)
	if err == nil {
		// 容器存在，执行快速重启复用
		fmt.Printf("[*] 检测到重名容器 %s (状态: %s)，执行快速重启复用...\n", containerName, info.State.Status)
		stopTimeout := 5 * time.Second
		err = cli.ContainerRestart(ctx, containerName, &stopTimeout)
		if err != nil {
			return "", fmt.Errorf("快速重启容器失败: %v", err)
		}
		fmt.Printf("[+] 出口容器 %s 重启成功\n", containerName)
		return info.ID, nil
	}

	// 2. 容器不存在，执行创建并启动流程
	fmt.Printf("[*] 正在创建出口容器 %s，宿主机映射端口: %d...\n", containerName, egress.Port)

	portBind := nat.PortMap{
		"10000/tcp": []nat.PortBinding{
			{HostPort: strconv.Itoa(egress.Port)},
		},
	}

	resp, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image: egressImage,
			ExposedPorts: nat.PortSet{
				"10000/tcp": struct{}{},
			},
		},
		&container.HostConfig{
			PortBindings: portBind,
			CapAdd:       []string{"NET_ADMIN"}, // OpenVPN 需要网络管理权限
			Resources: container.Resources{
				Devices: []container.DeviceMapping{
					{
						PathOnHost:        "/dev/net/tun",
						PathInContainer:   "/dev/net/tun",
						CgroupPermissions: "rwm",
					},
				},
			},
			Binds: []string{
				fmt.Sprintf("%s/client.ovpn:/etc/openvpn/client.ovpn:ro", hostEgressDir),
				fmt.Sprintf("%s/config.json:/etc/xray/config.json:ro", hostEgressDir),
			},
			RestartPolicy: container.RestartPolicy{
				Name: "unless-stopped",
			},
			NetworkMode: "vless-net",
		},
		nil, nil, containerName,
	)
	if err != nil {
		return "", fmt.Errorf("创建容器失败: %v", err)
	}

	err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		// 启动失败尝试清理残留
		_ = cli.ContainerRemove(ctx, resp.ID, types.ContainerRemoveOptions{Force: true})
		return "", fmt.Errorf("启动容器失败: %v", err)
	}

	fmt.Printf("[+] 出口容器 %s 启动成功，容器ID: %s\n", containerName, resp.ID)
	return resp.ID, nil
}

// StopAndRemoveContainer 停止并删除出口容器
func StopAndRemoveContainer(name string) error {
	cli, err := getDockerClient()
	if err != nil {
		return err
	}

	containerName := containerPrefix + name
	ctx := context.Background()

	info, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		if client.IsErrNotFound(err) {
			fmt.Printf("[*] 容器 %s 未创建或已不存在，无需删除\n", containerName)
			return nil
		}
		return err
	}

	fmt.Printf("[*] 正在停止出口容器 %s...\n", containerName)
	if info.State.Running {
		stopTimeout := 5 * time.Second
		err = cli.ContainerStop(ctx, containerName, &stopTimeout)
		if err != nil {
			fmt.Printf("[!] 停止容器 %s 失败: %v，尝试强制删除\n", containerName, err)
		}
	}

	fmt.Printf("[*] 正在删除出口容器 %s...\n", containerName)
	err = cli.ContainerRemove(ctx, containerName, types.ContainerRemoveOptions{Force: true})
	if err != nil {
		return fmt.Errorf("删除容器 %s 失败: %v", containerName, err)
	}

	fmt.Printf("[+] 出口容器 %s 删除成功\n", containerName)
	return nil
}

// GetContainerStatusAndIp 获取状态与 IP (含 6 秒缓存包装)
func GetContainerStatusAndIp(name string) (*ContainerStatusReport, error) {
	statusCacheMutex.RLock()
	cached, found := statusCache[name]
	if found && time.Since(cached.timestamp) < cacheMaxAge {
		defer statusCacheMutex.RUnlock()
		return cached.data, nil
	}
	statusCacheMutex.RUnlock()

	statusCacheMutex.Lock()
	defer statusCacheMutex.Unlock()
	// 双检锁
	if cached, found := statusCache[name]; found && time.Since(cached.timestamp) < cacheMaxAge {
		return cached.data, nil
	}

	report, err := getContainerStatusAndIpRaw(name)
	if err != nil {
		return nil, err
	}

	statusCache[name] = cachedReport{
		timestamp: time.Now(),
		data:      report,
	}
	return report, nil
}

// 容器测活获取外部 IP 的底层实现
func getContainerStatusAndIpRaw(name string) (*ContainerStatusReport, error) {
	cli, err := getDockerClient()
	if err != nil {
		return nil, err
	}

	containerName := containerPrefix + name
	ctx := context.Background()

	info, err := cli.ContainerInspect(ctx, containerName)
	if err != nil {
		if client.IsErrNotFound(err) {
			return &ContainerStatusReport{Status: "offline", IP: "offline", Error: "容器未创建或已被删除"}, nil
		}
		return nil, err
	}

	if !info.State.Running {
		return &ContainerStatusReport{Status: info.State.Status, IP: "offline", Error: "容器未运行"}, nil
	}

	// 简体中文注释：IP 探测源列表随机打散，执行多源回退测活，降低 429 被封禁概率
	ipApis := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://ipinfo.io/ip",
	}
	// 随机排序
	rand.Seed(time.Now().UnixNano())
	shuffled := make([]string, len(ipApis))
	copy(shuffled, ipApis)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	var cmdList []string
	for _, api := range shuffled {
		cmdList = append(cmdList, fmt.Sprintf("curl -fs --interface tun0 --max-time 4 %s", api))
	}
	cmdString := strings.Join(cmdList, " || ")

	// 容器内执行 Exec
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"sh", "-c", cmdString},
	}

	execCreateResp, err := cli.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return &ContainerStatusReport{Status: "running", IP: "error", Error: fmt.Sprintf("创建 Exec 失败: %v", err)}, nil
	}

	// 建立 Exec 连接并获取 ioStream
	resp, err := cli.ContainerExecAttach(ctx, execCreateResp.ID, types.ExecStartCheck{})
	if err != nil {
		return &ContainerStatusReport{Status: "running", IP: "error", Error: fmt.Sprintf("启动 Exec 失败: %v", err)}, nil
	}
	defer resp.Close()

	// 拆流缓冲区
	var stdoutBuf, stderrBuf bytes.Buffer
	doneChan := make(chan error, 1)

	go func() {
		// 简体中文注释：使用 StdCopy 安全处理非 TTY 容器多路复用流，规避包头二进制乱码，彻底净化输出
		_, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, resp.Reader)
		doneChan <- err
	}()

	select {
	case err := <-doneChan:
		if err != nil {
			return &ContainerStatusReport{Status: "error", IP: "error", Error: fmt.Sprintf("读取流出错: %v", err)}, nil
		}
	case <-time.After(10 * time.Second):
		return &ContainerStatusReport{Status: "running", IP: "error", Error: "获取出口 IP 动作执行超时"}, nil
	}

	cleanIP := strings.TrimSpace(stdoutBuf.String())
	if ipv4Regexp.MatchString(cleanIP) {
		return &ContainerStatusReport{Status: "running", IP: cleanIP, Error: ""}, nil
	}

	return &ContainerStatusReport{
		Status: "error",
		IP:     "error",
		Error:  fmt.Sprintf("无法获取有效出口 IP, stdout: %s, stderr: %s", cleanIP, strings.TrimSpace(stderrBuf.String())),
	}, nil
}

// GetContainerLogs 获取出口容器最近日志
func GetContainerLogs(name string, tail int) (string, error) {
	cli, err := getDockerClient()
	if err != nil {
		return "", err
	}

	containerName := containerPrefix + name
	ctx := context.Background()

	_, err = cli.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", fmt.Errorf("容器未创建或已被删除: %v", err)
	}

	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Tail:       strconv.Itoa(tail),
	}

	reader, err := cli.ContainerLogs(ctx, containerName, options)
	if err != nil {
		return "", fmt.Errorf("读取日志流失败: %v", err)
	}
	defer reader.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	// 拆流去除 Docker Stream 包头
	_, err = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, reader)
	if err != nil {
		return "", fmt.Errorf("解析日志包头失败: %v", err)
	}

	// 拼接错误与正常输出返回
	logs := stdoutBuf.String() + stderrBuf.String()
	if logs == "" {
		return "暂无日志", nil
	}
	return logs, nil
}
