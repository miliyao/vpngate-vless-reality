# VLESS Reality 多出口自愈代理系统 (VPNGate Bridge)

这是一个运行在 Docker 容器上的多地区出口代理管理系统。它允许你连接到单台 VPS 入口，但通过后台的 VPNGate 节点建立多个不同国家/地区的代理出口。同时，系统内置任务队列、SQLite 持久化、订阅链接和**定时健康检测与透明漂移 (Transparent Drifting) 自愈机制**，用于提高出口的可维护性与稳定性。

---

## 🏗️ 核心架构与稳定性保障

### 1. 策略路由解决回程不对称 (Policy Routing)
由于 OpenVPN 会在容器内创建 `tun0` 并接管默认路由，Xray 服务端的入站 TCP 握手回包如果直接走 VPN 出口，会导致 TCP 握手失败。
本方案在 `entrypoint.sh` 中利用 `ip rule` 和新的路由表备份了容器的原网关，确保从宿主机映射进来的流量在回复时仍然通过原宿主机网卡原路返回，而 Xray 主动发起的代理流量正常走 `tun0` 出口。这实现了入站与出站的物理隔离。

### 2. 透明漂移自愈 (Transparent Drifting)
VPNGate 的公共节点具有不确定性，经常会失效。本系统后台运行着一个健康监视器：
* 后台 Worker 每 60 秒对所有容器进行连通性（延迟与外网 IP）探测。
* 如果判定某个出口连续 2 次失效，Worker 将自动联网抓取该地区最新的可用节点，覆写配置文件并重启对应的出口容器。
* 由于 **Xray 的宿主机端口、UUID 客户端密钥、Reality 证书等完全保持不变**，客户端在节点漂移时无须做任何配置修改，即可在 5-15 秒内自动连通新 IP。

### 3. 管理任务队列
面板上的创建、删除、漂移和批量操作会先写入 SQLite 任务队列，再由独立 Worker 执行。这样 Web API 不会因为长任务阻塞，刷新页面后也能查看最近任务状态与错误详情。

---

## 📂 项目结构

```text
vpngate-vless-reality/
├── docker/
│   └── egress-image/
│       ├── Dockerfile              # 出口容器基础镜像
│       └── entrypoint.sh           # 出口网络启动及自愈脚本
├── web/
│   ├── backend/
│   │   ├── app.js                  # 控制面板 Express 入口
│   │   ├── worker.js               # 任务队列与自愈 Worker
│   │   ├── package.json            # 后端依赖配置
│   │   ├── routes/                 # 路由控制接口 (出口 CRUD、VPNGate 数据)
│   │   ├── services/               # 核心服务 (Dockerode控制、Reality密钥对生成)
│   │   └── models/                 # SQLite 数据库存储
│   └── frontend/
│       ├── src/
│       │   ├── App.vue             # 现代科技感仪表盘前端界面
│       │   └── index.css           # 纯手工精美 Vanilla CSS 暗黑主题
│       ├── index.html              # 前端模板入口
│       └── vite.config.js          # Vite 构建与代理配置
├── config/
│   └── xray-config.template.json   # Xray 服务端 Reality 配置模板
├── docker-compose.yml              # Web 面板与 Worker 编排文件
├── .env.example                    # 生产环境配置示例
├── CHANGELOG.md                    # 版本变更记录
└── README.md                       # 说明文档
```

---

## 🚀 快速部署指南

### 方法一：极简 Curl 一键部署 (最推荐)
在你的国外 VPS 上，无需手动下载代码，只需以 root 权限运行以下一行指令：
```bash
curl -fsSL https://raw.githubusercontent.com/miliyao/vpngate-vless-reality/main/deploy.sh | bash
```
该指令会自动安装所需系统依赖，克隆 GitHub 仓库，配置 Docker 与 Docker Compose，构建出口镜像并一键拉起控制面板。
部署脚本会自动生成 `.env`，并在完成时输出面板账号和密码。

---

### 方法二：Git 克隆后手动执行脚本
如果你已经把项目克隆到了本地或者 VPS 文件夹（例如 `/root/vless`），可以直接运行：
```bash
# 给予脚本执行权限并一键拉起
chmod +x deploy.sh
./deploy.sh
```

---

### 方法三：手动逐步部署
如果你希望手动控制部署的每一步，请依次运行以下命令：

#### 1. 构建出口镜像
```bash
# 构建出口容器的基础镜像（名称必须固定为 vpngate-egress:latest）
docker build -t vpngate-egress:latest ./docker/egress-image/
```

### 2. 构建前端静态资源
控制面板由 Node.js 后端静态托管打包后的前端。在启动面板前，需要对前端进行打包：
```bash
# 2. 进入前端目录，安装依赖并打包 (打包产物会自动写入 web/backend/dist)
cd web/frontend
npm install
npm run build
cd ../..
```

### 3. 配置环境变量
```bash
cp .env.example .env
nano .env
```

至少需要修改：
```bash
PANEL_PASSWORD=你的强密码
HOST_DATA_PATH=/当前项目绝对路径/data
```

常用配置：
```bash
PANEL_PORT=3000
PANEL_USERNAME=admin
RE_DOMAINS=www.amd.com
VPS_ADDRESS=
```

### 4. 一键拉起控制面板
在项目根目录下执行：
```bash
# 4. 运行 Docker Compose 拉起控制面板服务和后台 Worker
docker compose up -d
```

### 5. 访问面板与配置
* 打开浏览器访问：`http://你的服务器IP:3000`，输入 `.env` 中的 `PANEL_USERNAME` 和 `PANEL_PASSWORD`。
* 选择你需要的出口国家/地区，点击创建或一键生成多地区。
* 等待出口状态变为“运行中”后，复制单节点链接或订阅 URL，将其粘贴到客户端（如 v2rayN, Clash Meta, Sing-box）即可连接。
* **Reality 混淆 SNI 域名**：默认配置为 `www.amd.com`，可以通过 `.env` 中的 `RE_DOMAINS` 修改。
* 出口容器会动态监听 `44301-44400` 端口，请在 VPS 防火墙和云厂商安全组中放行该 TCP 端口段。

---

## 🔐 安全与备份

* 面板默认启用 HTTP Basic Auth。请部署后立即保存 `.env` 中的密码，不要将 `.env` 提交到 Git。
* 数据库位于 `data/app.sqlite3`，出口配置位于 `data/egress/`。迁移服务器前需要一起备份。
* 建议只向可信 IP 开放面板端口；出口端口段 `44301-44400/tcp` 需要对客户端开放。
* 如果前面有 Nginx/Caddy 反代，建议额外启用 HTTPS。

---

## 🧰 常用运维命令

```bash
# 查看服务状态
docker compose ps

# 查看面板日志
docker logs -f vless-web-panel

# 查看 Worker 日志
docker logs -f vless-self-healing-worker

# 修改 .env 后重启
docker compose up -d

# 备份数据
tar -czf vless-reality-backup-$(date +%F).tar.gz data .env
```
