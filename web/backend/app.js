// 控制面板后端服务主入口文件
// 中文注释，保障系统整体架构的鲁棒性，集成后台自动漂移自愈系统

const express = require('express');
const cors = require('cors');
const path = require('path');
const fs = require('fs');

const egressRouter = require('./routes/egress');
const vpngateRouter = require('./routes/vpngate');
const db = require('./models/db');
const dockerService = require('./services/docker');
const vpngateFetcher = require('./services/vpngate-fetcher');

const app = express();
const PORT = process.env.PORT || 3000;

// 1. 中间件配置
app.use(cors());
app.use(express.json());
app.use(express.urlencoded({ extended: true }));

// 2. 路由分发
app.use('/api/egress', egressRouter);
app.use('/api/vpngate', vpngateRouter);

// 3. 静态文件托管（Vite 打包输出目录为后端根目录下的 dist 文件夹）
const FRONTEND_DIST = path.join(__dirname, 'dist');
if (fs.existsSync(FRONTEND_DIST)) {
  app.use(express.static(FRONTEND_DIST));
  // 单页面路由回退：非 API 请求一律返回 index.html
  app.get('*', (req, res, next) => {
    if (req.path.startsWith('/api')) {
      return next();
    }
    res.sendFile(path.join(FRONTEND_DIST, 'index.html'));
  });
} else {
  // 提示前端尚未编译
  app.get('/', (req, res) => {
    res.send('<h1 style="text-align:center;margin-top:100px;font-family:sans-serif;">VLESS Reality Egress Panel Backend is running.<br>前端页面尚未打包编译，请进入 web/frontend 执行 npm run build。</h1>');
  });
}

// 4. 后台自动健康检查与透明漂移守护任务 (Self-Healing Daemon)
// 每 60 秒轮询检测所有出口的运行状况
const HEALTH_CHECK_INTERVAL = 60000;
// 判定失效的最大容忍次数，连续 2 次失败则触发自动漂移
const MAX_FAILURES = 2;
// 内存中记录每个出口的连续失败次数
const failureTracker = {};

function startSelfHealingDaemon() {
  console.log('[*] 自动漂移自愈守护进程已启动...');
  
  setInterval(async () => {
    const egresses = db.getEgresses();
    
    for (const egress of egresses) {
      const name = egress.name;
      
      // 如果出口已被标记为暂停或删除中，则跳过
      if (egress.status === 'starting') {
        continue;
      }

      try {
        const statusReport = await dockerService.getContainerStatusAndIp(name);
        
        if (statusReport.status !== 'running' || statusReport.ip === 'error') {
          // 累加失败计数
          failureTracker[name] = (failureTracker[name] || 0) + 1;
          console.warn(`[!] 出口 ${name} 检测异常 (${failureTracker[name]}/${MAX_FAILURES})，错误原因: ${statusReport.error || '连通性断开'}`);

          // 当连续失败次数达到最大容忍限制，触发自动漂移
          if (failureTracker[name] >= MAX_FAILURES) {
            console.error(`[!] 触发自愈！出口 ${name} 已连续 ${MAX_FAILURES} 次健康检查失败，正在执行自动透明漂移...`);
            
            // 重置计数，避免重复触发
            failureTracker[name] = 0;
            db.updateEgress(name, { status: 'starting' });

            // 开始漂移逻辑：拉取新节点 -> 更新 ovpn -> 重启容器
            try {
              const bestNode = await vpngateFetcher.getBestNode(egress.region);
              
              const egressDir = path.join(__dirname, 'data', 'egress', name);
              if (!fs.existsSync(egressDir)) {
                fs.mkdirSync(egressDir, { recursive: true });
              }
              fs.writeFileSync(path.join(egressDir, 'client.ovpn'), bestNode.ovpnConfig, 'utf-8');
              
              db.updateEgress(name, {
                nodeIp: bestNode.ip,
                nodeHostname: bestNode.hostname,
                latency: bestNode.ping
              });

              // 启动新容器
              await dockerService.startEgressContainer(egress);
              db.updateEgress(name, { status: 'running' });
              console.log(`[+] [自愈成功] 出口 ${name} 已成功漂移至新节点 IP: ${bestNode.ip}`);
            } catch (driftErr) {
              db.updateEgress(name, { status: 'error', error: `自愈失败: ${driftErr.message}` });
              console.error(`[-] [自愈失败] 出口 ${name} 透明漂移出错:`, driftErr.message);
            }
          }
        } else {
          // 检测通过，重置失败计数
          if (failureTracker[name] > 0) {
            console.log(`[+] 出口 ${name} 网络已恢复，重置计数`);
          }
          failureTracker[name] = 0;
        }
      } catch (err) {
        console.error(`[-] 定时检测出口 ${name} 时发生系统错误:`, err.message);
      }
    }
  }, HEALTH_CHECK_INTERVAL);
}

// 5. 启动服务并开启守护进程
app.listen(PORT, () => {
  console.log(`[+] 控制面板后端服务已在端口 ${PORT} 启动！`);
  startSelfHealingDaemon();
});
