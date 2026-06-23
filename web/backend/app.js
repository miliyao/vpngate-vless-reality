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
const PANEL_USERNAME = process.env.PANEL_USERNAME || '';
const PANEL_PASSWORD = process.env.PANEL_PASSWORD || '';
const AUTH_ENABLED = process.env.PANEL_AUTH_ENABLED !== 'false' && Boolean(PANEL_USERNAME && PANEL_PASSWORD);

function safeEqual(a, b) {
  const left = Buffer.from(a || '');
  const right = Buffer.from(b || '');
  return left.length === right.length && require('crypto').timingSafeEqual(left, right);
}

function requirePanelAuth(req, res, next) {
  if (!AUTH_ENABLED) return next();

  const header = req.headers.authorization || '';
  const [scheme, encoded] = header.split(' ');
  if (scheme === 'Basic' && encoded) {
    const decoded = Buffer.from(encoded, 'base64').toString('utf8');
    const separator = decoded.indexOf(':');
    const username = separator >= 0 ? decoded.slice(0, separator) : '';
    const password = separator >= 0 ? decoded.slice(separator + 1) : '';
    if (safeEqual(username, PANEL_USERNAME) && safeEqual(password, PANEL_PASSWORD)) {
      return next();
    }
  }

  res.set('WWW-Authenticate', 'Basic realm="VLESS Reality Panel", charset="UTF-8"');
  if (req.path.startsWith('/api')) {
    return res.status(401).json({ error: '未授权' });
  }
  return res.status(401).send('Unauthorized');
}

// 1. 中间件配置
app.use(cors());
app.use(express.json());
app.use(express.urlencoded({ extended: true }));
app.use(requirePanelAuth);

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
