// 控制面板后端服务主入口文件
// 中文注释，保障系统整体架构的鲁棒性，集成后台自动漂移自愈系统

const express = require('express');
const cors = require('cors');
const path = require('path');
const fs = require('fs');

const egressRouter = require('./routes/egress');
const vpngateRouter = require('./routes/vpngate');

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

// 4. 启动服务
app.listen(PORT, () => {
  console.log(`[+] 控制面板后端服务已在端口 ${PORT} 启动！`);
});
