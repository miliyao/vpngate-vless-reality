// 封装 Dockerode 服务，负责管理出口 Egress 容器的生命周期
// 中文注释，保障逻辑清晰及错误处理

const Docker = require('dockerode');

// 初始化 Docker 客户端，读取挂载的 docker.sock
const docker = new Docker({ socketPath: '/var/run/docker.sock' });

// 镜像名称
const EGRESS_IMAGE = 'vpngate-egress:latest';

// 缓存宿主机数据目录路径，避免每次启动容器都执行 inspect
let cachedHostDataPath = null;

/**
 * 推导宿主机上 /app/data 挂载的真实绝对路径
 * 优先使用环境变量 HOST_DATA_PATH，否则通过 inspect 面板自身容器获取
 */
async function resolveHostDataPath() {
  // 如果已有缓存，直接返回
  if (cachedHostDataPath) return cachedHostDataPath;

  // 方式一：通过环境变量显式指定（docker-compose 中配置）
  if (process.env.HOST_DATA_PATH) {
    cachedHostDataPath = process.env.HOST_DATA_PATH;
    console.log(`[+] 从环境变量获取宿主机 Data 路径: ${cachedHostDataPath}`);
    return cachedHostDataPath;
  }

  // 方式二：通过 Docker inspect 面板自身容器，自动探测绑定挂载源路径
  try {
    const selfContainer = docker.getContainer('vless-web-panel');
    const selfInfo = await selfContainer.inspect();
    const dataMount = selfInfo.Mounts.find(m => m.Destination === '/app/data');
    if (dataMount && dataMount.Source) {
      cachedHostDataPath = dataMount.Source;
      console.log(`[+] 自动探测到宿主机 Data 真实路径: ${cachedHostDataPath}`);
      return cachedHostDataPath;
    }
  } catch (err) {
    console.warn('[!] 自动探测宿主机挂载路径失败:', err.message);
  }

  // 方式三：回退方案
  cachedHostDataPath = '/app/data';
  console.warn('[!] 未能推导宿主机路径，使用容器内路径 /app/data（在 docker.sock 模式下可能导致挂载失败）');
  return cachedHostDataPath;
}

/**
 * 启动或创建出口容器
 * @param {Object} egress - 出口对象
 * @param {string} egress.name - 出口名称
 * @param {number} egress.port - 宿主机暴露端口
 */
async function startEgressContainer(egress) {
  const containerName = `egress-${egress.name}`;

  const hostDataPath = await resolveHostDataPath();
  const hostEgressDir = `${hostDataPath}/egress/${egress.name}`;

  // 1. 检查同名容器是否已存在，如果存在则先停止并删除
  try {
    const existingContainer = docker.getContainer(containerName);
    const info = await existingContainer.inspect();
    console.log(`[*] 检测到重名容器 ${containerName} (状态: ${info.State.Status})，正在进行清理...`);
    if (info.State.Running) {
      await existingContainer.stop({ t: 5 });
    }
    await existingContainer.remove();
    console.log(`[+] 旧容器 ${containerName} 清理完毕`);
  } catch (error) {
    // 容器不存在，忽略
  }

  // 2. 创建并启动新容器
  console.log(`[*] 正在创建出口容器 ${containerName}，宿主机映射端口: ${egress.port}...`);
  
  const container = await docker.createContainer({
    Image: EGRESS_IMAGE,
    name: containerName,
    HostConfig: {
      PortBindings: {
        '10000/tcp': [{ HostPort: String(egress.port) }]
      },
      CapAdd: ['NET_ADMIN'], // OpenVPN 需要网络管理权限
      Devices: [
        {
          PathOnHost: '/dev/net/tun',
          PathInContainer: '/dev/net/tun',
          CgroupPermissions: 'rwm'
        }
      ],
      Binds: [
        `${hostEgressDir}/client.ovpn:/etc/openvpn/client.ovpn:ro`,
        `${hostEgressDir}/config.json:/etc/xray/config.json:ro`
      ],
      // 由 self-healing.js 统一决策是否漂移，容器只需保证始终可重启。
      // 使用 unless-stopped 策略避免 on-failure 重试上限耗尽后容器永久停止。
      RestartPolicy: { Name: 'unless-stopped' },
      // 将出口容器加入面板同一网络，便于后续内部通信扩展
      NetworkMode: 'vless-net'
    }
  });

  await container.start();
  console.log(`[+] 出口容器 ${containerName} 启动成功，容器ID: ${container.id}`);
  return container.id;
}

/**
 * 停止并删除出口容器
 * @param {string} name - 出口名称
 */
async function stopAndRemoveContainer(name) {
  const containerName = `egress-${name}`;
  try {
    const container = docker.getContainer(containerName);
    const info = await container.inspect();
    
    console.log(`[*] 正在停止出口容器 ${containerName}...`);
    if (info.State.Running) {
      await container.stop({ t: 5 });
    }
    
    console.log(`[*] 正在删除出口容器 ${containerName}...`);
    await container.remove();
    console.log(`[+] 出口容器 ${containerName} 删除成功`);
    return true;
  } catch (error) {
    console.log(`[*] 容器 ${containerName} 未运行或不存在，无需删除`);
    return false;
  }
}

/**
 * 检查容器状态并获取当前 VPN 出口的真实 IP
 * 使用 Dockerode 原生的 demuxStream 正确解析 Docker 多路复用 stream 协议头
 * @param {string} name - 出口名称
 */
async function getContainerStatusAndIp(name) {
  const containerName = `egress-${name}`;
  try {
    const container = docker.getContainer(containerName);
    const info = await container.inspect();
    const isRunning = info.State.Running;
    const status = info.State.Status; // running, exited, etc.

    if (!isRunning) {
      return { status, ip: 'offline', error: '容器未运行' };
    }

    // 简体中文注释：采用多源容错与随机打散，防止单点故障及 429 速率限制
    const ipApis = [
      'https://api.ipify.org',
      'https://ifconfig.me/ip',
      'https://ipinfo.io/ip'
    ];
    const shuffledApis = [...ipApis].sort(() => Math.random() - 0.5);
    const cmdString = shuffledApis.map(api => `curl -fs --interface tun0 --max-time 4 ${api}`).join(' || ');

    // 在容器内部执行 curl 获取 VPN 出口真实 IP，支持多源回退与防限流
    try {
      const exec = await container.exec({
        Cmd: ['sh', '-c', cmdString],
        AttachStdout: true,
        AttachStderr: true
      });

      const stream = await exec.start({ Detach: false });
      
      return new Promise((resolve) => {
        let stdout = '';
        let stderr = '';

        // 使用 Dockerode 官方提供的 demuxStream 工具方法
        // 正确分离 stdout/stderr 的多路复用协议帧（每帧前 8 字节为协议头）
        const { PassThrough } = require('stream');
        const stdoutStream = new PassThrough();
        const stderrStream = new PassThrough();

        docker.modem.demuxStream(stream, stdoutStream, stderrStream);

        stdoutStream.on('data', (chunk) => {
          stdout += chunk.toString();
        });
        stderrStream.on('data', (chunk) => {
          stderr += chunk.toString();
        });

        // 设置超时保护，避免 exec 挂起导致健康检查阻塞
        const timeout = setTimeout(() => {
          resolve({ status: 'running', ip: 'error', error: '容器内 IP 检测超时' });
        }, 10000);

        stream.on('end', () => {
          clearTimeout(timeout);
          const cleanIp = stdout.trim();
          
          // 验证 IP 格式是否为标准 IPv4
          if (cleanIp && /^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(cleanIp)) {
            resolve({ status: 'running', ip: cleanIp });
          } else {
            resolve({ status: 'error', ip: 'error', error: `无法获取有效出口 IP，stdout: ${cleanIp}, stderr: ${stderr.trim()}` });
          }
        });

        stream.on('error', (err) => {
          clearTimeout(timeout);
          resolve({ status: 'error', ip: 'error', error: err.message });
        });
      });
    } catch (execErr) {
      return { status: 'running', ip: 'error', error: `执行 IP 检测失败: ${execErr.message}` };
    }
  } catch (error) {
    return { status: 'offline', ip: 'offline', error: '容器未创建或已被删除' };
  }
}

/**
 * 获取出口容器最近日志，便于面板诊断 OpenVPN / Xray 启动失败原因
 * @param {string} name - 出口名称
 * @param {number} tail - 返回最近多少行日志
 */
async function getContainerLogs(name, tail = 120) {
  const containerName = `egress-${name}`;
  try {
    const container = docker.getContainer(containerName);
    await container.inspect();

    const logBuffer = await container.logs({
      stdout: true,
      stderr: true,
      timestamps: true,
      tail
    });

    // docker logs 在非 TTY 容器下会带 8 字节 stream header，这里做一次轻量清理。
    let offset = 0;
    const chunks = [];
    while (offset + 8 <= logBuffer.length) {
      const frameSize = logBuffer.readUInt32BE(offset + 4);
      const frameStart = offset + 8;
      const frameEnd = frameStart + frameSize;
      if (frameEnd > logBuffer.length) break;
      chunks.push(logBuffer.subarray(frameStart, frameEnd));
      offset = frameEnd;
    }

    if (chunks.length > 0) {
      return Buffer.concat(chunks).toString('utf-8');
    }
    return logBuffer.toString('utf-8');
  } catch (error) {
    throw new Error(`读取容器日志失败: ${error.message}`);
  }
}

module.exports = {
  startEgressContainer,
  stopAndRemoveContainer,
  getContainerStatusAndIp,
  getContainerLogs
};
