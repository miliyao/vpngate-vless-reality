# VLESS Reality 多出口自愈代理系统 (VPNGate Bridge)

这是一个运行在 Docker 容器上的多地区出口代理管理系统。它允许你连接到单台国外 VPS 入口，但通过后台的 VPNGate 节点池建立多个不同的国家/地区代理出口。同时，系统内置了**定时健康检测与透明漂移 (Transparent Drifting) 自愈机制**，确保出口的高可用性与稳定性。

---

## 🏗️ 核心架构与稳定性保障

### 1. 策略路由解决回程不对称 (Policy Routing)
由于 OpenVPN 会在容器内创建 `tun0` 并接管默认路由，Xray 服务端的入站 TCP 握手回包如果直接走 VPN 出口，会导致 TCP 握手失败。
本方案在 `entrypoint.sh` 中利用 `ip rule` 和新的路由表备份了容器的原网关，确保从宿主机映射进来的流量在回复时仍然通过原宿主机网卡原路返回，而 Xray 主动发起的代理流量正常走 `tun0` 出口。这实现了入站与出站的物理隔离。

### 2. 透明漂移自愈 (Transparent Drifting)
VPNGate 的公共节点具有不确定性，经常会失效。本系统后台运行着一个健康监视器：
* 后端主控每 60 秒对所有容器进行连通性（延迟与外网 IP）探测。
* 如果判定某个出口连续 2 次失效，主控将自动联网抓取该地区最新的最佳节点，覆写配置文件并重启对应的出口容器。
* 由于 **Xray 的宿主机端口、UUID 客户端密钥、Reality 证书等完全保持不变**，客户端在节点漂移时无须做任何配置修改，即可在 5-15 秒内自动连通新 IP。

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
│   │   ├── app.js                  # 控制面板 Express 入口，集成自愈守护进程
│   │   ├── package.json            # 后端依赖配置
│   │   ├── routes/                 # 路由控制接口 (出口 CRUD、VPNGate 数据)
│   │   ├── services/               # 核心服务 (Dockerode控制、Reality密钥对生成)
│   │   └── models/                 # 极简 JSON 数据库存储
│   └── frontend/
│       ├── src/
│       │   ├── App.vue             # 现代科技感仪表盘前端界面
│       │   └── index.css           # 纯手工精美 Vanilla CSS 暗黑主题
│       ├── index.html              # 前端模板入口
│       └── vite.config.js          # Vite 构建与代理配置
├── config/
│   └── xray-config.template.json   # Xray 服务端 Reality 配置模板
├── docker-compose.yml              # 统一一键编排文件
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

---

### 方法二：Git 克隆后手动执行脚本
如果你已经把项目克隆到了本地或者 VPS 文件夹（例如 `/root/vless`），可以直接运行：
```bash
# 给予脚本执行权限并一键拉起
chmod +x deploy.sh
./deploy.sh
```

---

### 方法二：手动逐步部署
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

### 3. 一键拉起控制面板
在项目根目录下执行：
```bash
# 3. 运行 Docker Compose 拉起控制面板服务
docker compose up -d
```

### 4. 访问面板与配置
* 打开浏览器访问：`http://你的服务器IP:3000` 即可进入管理面板。
* 输入出口名称，选择你需要的出口国家（如 `JP`, `US`, `KR` 等），点击 **一键构建出口**。
* 等待容器状态变为“运行中”后，点击 **复制 VLESS 订阅**，将其粘贴到客户端（如 v2rayN, Clash Meta, Sing-box）即可直接连接。
* **Reality 混淆 SNI 域名**：默认配置已将 `www.asus.com` 用于握手混淆，你可以通过修改 `docker-compose.yml` 中的 `RE_DOMAINS` 环境变量来更改为您需要的安全域名。
* 出口容器会动态监听 `44301-44400` 端口，请在 VPS 防火墙和云厂商安全组中放行该 TCP 端口段。
