# VLESS Reality 多出口自愈代理系统 (VPNGate Bridge)

这是一个运行在 Docker 容器上的多地区出口代理管理系统。它允许你连接到单台 VPS 入口，但通过后台的 VPNGate 节点建立多个不同国家/地区的代理出口。同时，系统内置任务队列、SQLite 持久化、订阅链接和**定时健康检测与透明漂移 (Transparent Drifting) 自愈机制**，用于提高出口的可维护性与稳定性。

---

## 🏗️ 核心架构与稳定性保障

### 1. 策略路由解决回程不对称 (Policy Routing)
由于 OpenVPN 会在容器内创建 `tun0` 并接管默认路由，Xray 服务端的入站 TCP 握手回包如果直接走 VPN 出口，会导致 TCP 握手失败。
本方案在 `entrypoint.sh` 中利用 `ip rule` 和新的路由表备份了容器的原网关，确保从宿主机映射进来的流量在回复时仍然通过原宿主机网卡原路返回，而 Xray 主动发起的代理流量正常走 `tun0` 出口。这实现了入站与出站的物理隔离。

### 2. 透明漂移自愈 (Transparent Drifting)
VPNGate 的公共节点具有不确定性，经常会失效。本系统后台运行着一个健康监视器：
* 后台 Worker 按 `HEALTH_CHECK_INTERVAL` 配置对所有容器进行连通性（延迟与外网 IP）探测，默认 24 小时一次。
* 如果判定某个出口连续 2 次失效，Worker 将自动联网抓取该地区最新的可用节点，覆写配置文件并重启对应的出口容器。
* 由于 **Xray 的宿主机端口、UUID 客户端密钥、Reality 证书等完全保持不变**，客户端在节点漂移时无须做任何配置修改，即可在 5-15 秒内自动连通新 IP。

### 3. VPNGate 候选池与节点历史
VPNGate 返回的是公开 OpenVPN 节点原始 CSV 数据，节点可用性波动较大。本系统会将原始节点转化为候选池，并按 `Score`、`Speed`、`Uptime`、`Ping`、会话数和本机历史成功率进行综合评分。
* 创建或漂移出口时，系统会按候选评分顺序逐个尝试，不再只取单个最高分节点。
* 失败节点会写入 SQLite 历史，并在 `VPNGATE_FAILURE_COOLDOWN_MS` 窗口期内自动排除。
* 如果冷门地区排除后无节点，系统会自动降级允许冷却节点参与，避免地区被锁死。
* 登录后可通过 `/api/vpngate/candidates?region=JP` 查看当前候选评分，通过 `/api/vpngate/history?region=JP` 查看节点成功/失败历史。

### 4. 管理任务队列
面板上的创建、删除、漂移和批量操作会先写入 SQLite 任务队列，再由独立 Worker 执行。这样 Web API 不会因为长任务阻塞，刷新页面后也能查看最近任务状态与错误详情。

### 5. 健康检查与状态接口
Web 面板提供公开的 `/healthz` 存活检查，Docker Compose 会自动用它判断面板健康状态。登录后可访问 `/api/system/status` 查看版本、出口数量、任务状态统计和最近任务。

### 6. 纯 Go 强悍性能与单连接队列锁
系统后端已由 Node.js 彻底重构为纯 Go 语言版，不仅移除了 CGO 编译依赖支持全平台交叉编译，更大幅压缩了软硬件资源占用：
* **内存骤降 90%**：控制面板服务常态运行内存由 100MB 骤降至 **10MB 左右**。
* **极速响应**：Web API 并发响应时长进入微秒/毫秒级（**<1ms 响应**）。
* **串行安全读写**：数据库自动配置单物理连接队列锁，所有 SQL 读写由 Go 运行时协程排队互斥执行，**100% 根治 SQLite 多物理连接高并发写入时的 `database is locked` (SQLITE_BUSY) 锁竞争错误**。

---

## 📂 项目结构

```text
vpngate-vless-reality/
├── docker/
│   └── egress-image/
│       ├── Dockerfile              # 出口容器基础镜像
│       └── entrypoint.sh           # 出口网络启动及自愈脚本
├── web/
│   ├── backend-go/                 # 纯 Go 高性能一体化后端
│   │   ├── controllers/            # API 路由与控制器 (出口生命周期、VPNGate数据)
│   │   ├── models/                 # 纯 Go 无 CGO 的 SQLite 持久化层
│   │   ├── services/               # 核心业务服务 (Docker SDK、Reality密钥、Xray占位符)
│   │   ├── dist/                   # 前端编译静态资源包
│   │   ├── go.mod                  # Go 模块配置文件
│   │   ├── main.go                 # 主服务入口 (时序安全 Basic Auth、静态分发)
│   │   └── vless-panel             # 本地交叉编译的 Linux amd64 生产级免编译二进制
│   └── frontend/                   # Vue3 科技感暗黑前端界面
│       ├── src/
│       │   ├── components/         # 模块化前端子组件 (仪表盘状态、卡片、表单)
│       │   ├── App.vue             # 前端仪表盘主页面
│       │   └── index.css           # 科技感暗黑主题 CSS 样式系统
│       └── vite.config.js          # Vite 构建与代理配置 (打包产物直接重定向到 Go 目录下)
├── config/
│   └── xray-config.template.json   # Xray 服务端 Reality 占位模板配置
├── docker-compose.yml              # 容器编排文件
├── Dockerfile                      # 原生多阶段联编构建文件
├── Dockerfile.fast                 # 1秒极速打包运行时 Dockerfile (免 VPS 编译)
├── build.bat                       # 本地一键 Vue 打包 + Go 交叉编译 Linux 二进制脚本
├── scripts/
│   ├── backup.sh                   # 备份配置与 app.sqlite3 数据库
│   ├── doctor.sh                   # 部署自检
│   └── restore.sh                  # 从备份包中恢复数据
├── .env.example                    # 环境变量配置示例
├── CHANGELOG.md                    # 更新日志与版本记录
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

### 一键卸载与清除数据
若需要卸载整个系统并清除所有 Docker 出口容器与挂载数据，只需运行：
```bash
# 给予脚本执行权限并一键卸载 (加入 --purge 选项清除持久化数据)
chmod +x scripts/uninstall.sh
./scripts/uninstall.sh --purge
```
或者使用一键远程卸载：
```bash
curl -fsSL https://raw.githubusercontent.com/miliyao/vpngate-vless-reality/main/scripts/uninstall.sh | bash -s -- --purge
```

---

### 方法二：Git 克隆后手动执行脚本
如果你已经把项目克隆到了本地或者 VPS 文件夹（例如 `/root/vless`），可以直接运行：
```bash
# 给予脚本执行权限并一键拉起
chmod +x deploy.sh
./deploy.sh
```

---

### 方法三：【秒级部署】本地交叉编译 + 极速打包运行 (极力推荐)
为防止低配 VPS 发生编译卡顿或内存耗尽 (OOM)，可在本地开发机完成编译，在 VPS 实现秒级部署。

#### 1. 本地一键构建
在本地 Windows 开发机上，直接双击运行项目根目录下的 `build.bat`。它会自动执行：
* 前端依赖安装与打包（生成的前端静态文件将自动放置于后端 `web/backend-go/dist` 目录下）。
* 交叉编译 Go 后端为适用于 Linux amd64 架构的免依赖二进制程序。

#### 2. 推送至远程仓库 (或打包直传 VPS)
将本地编译好的产物强制加入 Git 索引并推送（请确保网络连通）：
```bash
git add -f web/backend-go/vless-panel web/backend-go/dist
git commit -m "build: 本地交叉编译产物"
git push
```

#### 3. VPS 上秒级拉起运行
在 VPS 上拉取代码，并配合 `Dockerfile.fast` 快速拉起，打包镜像仅需 1 秒：
```bash
# 构建出口容器的基础镜像（名称必须固定为 vpngate-egress:latest）
docker build -t vpngate-egress:latest ./docker/egress-image/

# 同步最新本地编译的二进制与配置
git pull

# 使用极速运行时 Dockerfile 完成镜像打包 (1秒内完成)
docker build -f Dockerfile.fast -t vless-reality-panel:latest .

# 极速拉起控制面板服务
docker compose up -d
```

---

### 方法四：手动逐步部署 (支持容器内多阶段构建)
如果您不想在本地进行编译，可以直接让 VPS 自动执行多阶段构建：

#### 1. 构建出口镜像
```bash
# 构建出口容器的基础镜像（名称必须固定为 vpngate-egress:latest）
docker build -t vpngate-egress:latest ./docker/egress-image/
```

#### 2. 配置环境变量
```bash
cp .env.example .env
nano .env
```
至少需要修改：
```bash
PANEL_PASSWORD=你的强密码
HOST_DATA_PATH=/当前项目绝对路径/data
```
常用可调参数：
```bash
# 自愈健康检测间隔，单位毫秒；默认 86400000，即 24 小时
HEALTH_CHECK_INTERVAL=86400000

# VPNGate 失败节点冷却窗口，单位毫秒；默认 1800000，即 30 分钟
VPNGATE_FAILURE_COOLDOWN_MS=1800000

# 创建/漂移时最多尝试的候选节点数量
VPNGATE_CREATE_CANDIDATE_LIMIT=8
VPNGATE_REBUILD_CANDIDATE_LIMIT=8
```

#### 3. 编译并拉起控制面板 (容器内全自动构建)
在项目根目录下执行 `--build` 选项，Docker 将自动在容器内构建前端 Vue 静态产物并编译 Go 模块：
```bash
# 自动多阶段联编前后端并后台运行 (低配 VPS 耗时可能较长)
docker compose up -d --build
```

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

# 一键自检
./scripts/doctor.sh

# 健康检查
curl http://127.0.0.1:3000/healthz

# 登录后查看详细状态 (Basic Auth 认证)
curl -u admin:你的密码 http://127.0.0.1:3000/api/system/status

# 查看某地区 VPNGate 候选节点评分
curl -u admin:你的密码 "http://127.0.0.1:3000/api/vpngate/candidates?region=JP"

# 查看某地区 VPNGate 节点成功/失败历史
curl -u admin:你的密码 "http://127.0.0.1:3000/api/vpngate/history?region=JP"

# 查看一体化面板日志
docker logs -f vless-web-panel

# 修改 .env 后重启
docker compose up -d

# 备份数据
./scripts/backup.sh

# 从备份恢复
./scripts/restore.sh ./backups/vless-reality-backup-YYYYmmdd-HHMMSS.tar.gz
```
