# 更新日志

## v1.2.0 - 2026-06-24

### 重构优化
- **高可用测活 IP 多源随机打散**：重构 `docker.js` 中的 `getContainerStatusAndIp` 容器测活功能，弃用对单个 `ipinfo.io` 测活接口的依赖，改为在 Node 端实现主流 IP API 的随机打散，并通过 `sh -c` 执行多源回退获取。此举极大分摊了高频探测对单点接口产生的请求负荷，消除了 429 速率限制引发的误判自愈。
- **Worker 任务互斥并发调度**：在 `worker.js` 中引入基于 `limitConcurrency` 的并发任务执行限制（最大并发度为 2），并结合基于 `target` 的互斥锁逻辑，有效防止同出口的并发操作导致容器竞态，大幅提升后台队列消费效率。
- **SQLite Job 序列化缺陷修复**：修复 `db.js` 中 `updateJob` 将已序列化的 JSON TEXT 二次 `JSON.stringify` 编码的缺陷，确保 `result`/`payload` 在持久化层中的 JSON 格式正确且解析无误。
- **数据库增量安全更新**：重构 `db.js` 中的 `updateEgress`，根据字段动态编译并缓存 SQL，杜绝 Read-Modify-Write 并发竞态条件，清理冗余 SQL。
- **智能自愈规避死节点**：重构 `vpngate-fetcher.js` 和 `egress-ops.js`，支持排除列表。自愈重建时自动隔离前一次连接失败的节点 IP 并更换新节点重试，彻底打破因死节点排名靠前而导致的无限重建死循环。
- **自愈机制并发化与异步解耦**：重构 `self-healing.js`。自愈状态检测由串行升级为 `Promise.allSettled` 并发，并将耗时的容器重建操作提交至任务队列作为异步 Job 处理，消除了自愈进程卡死风险，并使自动漂移进度和日志可在前台 Task 队列中直观展现，极大提升系统可观测性。
- **出口自愈探测去限流化**：在 `entrypoint.sh` 中使用各大科技公司无流量 Generate 204 HEAD 探测，替代容易引起 429 限流的 ipinfo.io 与 api.ipify.org 轮询检测，并优化获取 IP 的多源回退容错机制。
- **VPNGate API 多源自动兜底**：重构 `vpngate-fetcher.js`，支持配置多个备用 API 镜像源 URL。拉取节点失败时自动顺次切换至备选镜像源重试，彻底消除上游单点网络故障隐患。
- **批量操作受控并发化**：在 `egress-ops.js` 中引入轻量级 `limitConcurrency` 异步控制，将创建、重建和删除操作由串行升级为并发度为 3 的受控并发。
- **严格的安全路径防穿越**：对出口相关目录强制使用 `path.resolve` 进行绝对路径计算，严格判定目录前缀以防止利用恶意穿越字符对系统敏感文件进行破坏。
- **前端模块化组件拆分**：将 720 行单文件 `App.vue` 大幅拆分为 `SystemStatus.vue`、`CreateEgressForm.vue`、`EgressCard.vue` 和 `JobList.vue` 四个功能子组件，重构前后端组件通信，提高可维护性。
- **防爆心跳轮询与可见性监听**：将前端 `setInterval` 改为递归 `setTimeout`，并监听 `document.visibilitychange`，在标签页切入后台时暂停全部轮询以节省客户端/服务端开销，恢复可见时即刻同步唤醒。
- **前端 API 请求统一封装**：提取 `apiFetch` 拦截包装，统一规范 JSON Header 并全局捕获处理 API 内部抛出的 error。

### 部署集成
- **避免多服务重复编译与堆内存限制**：重构 `docker-compose.yml`，使 `self-healing-worker` 镜像直接继承复用 `web-panel` 编译后的镜像，消除低配 VPS 双重前端打包造成的 CPU 与内存开销。同时，在 `Dockerfile` 前端编译命令中加入限制堆内存参数 (`--max-old-space-size=512`)，彻底解决低配服务器部署时卡死的问题。
- **Docker 多阶段构建与 dist 移出 Git**：不再在 Git 中提交编译后的 `dist/` 静态产物，在 `docker-compose.yml` 中调整构建上下文到根目录，并在 `Dockerfile` 引入 Multi-stage Build 容器内自动编译，实现极致一键部署。
- **过滤规则优化**：项目根目录新增 `.dockerignore`，并在 `.gitignore` 补充编译排除，规避无关的 node_modules 和 data 传入镜像。

## v1.1.0 - 2026-06-24

### 重构优化
- **修复 worker 竞态**：`setInterval` 改为递归 `setTimeout`，彻底消除 Job 双重执行风险
- **list 接口并发化**：`Promise.allSettled` 替代串行 `for await`，接口延迟由 O(N) 降为 O(1)
- **failureCount 持久化**：自愈失败计数改由 SQLite 存储，worker 重启后不再清零
- **优雅停止**：出口容器 `entrypoint.sh` 新增 SIGTERM/SIGINT 信号捕获，确保子进程正确退出
- **缓存 TTL**：VPNGate 节点缓存加 1 小时过期，漂移时不再使用陈旧节点
- **移除 axios**：改用 Node 18 内置 `fetch`，减少一个外部依赖
- **模板内存缓存**：`xray-config-gen.js` 模板内容模块级缓存，批量建出口时不重复读磁盘
- **重启策略修正**：出口容器从 `on-failure:3` 改为 `unless-stopped`，将漂移决策权统一交给自愈服务
- **启动顺序依赖**：worker 新增 `depends_on web-panel: service_healthy`，消除 SQLite 初始化竞态
- **Xray 日志 stderr 化**：日志改写 stderr，由 Docker 统一轮转，不再累积容器内文件
- **镜像版本固定**：后端 Dockerfile 锁定 `node:18.20-slim`，确保构建可复现
- **备份保留策略**：`backup.sh` 自动清理超出 7 份的旧备份（可通过 `BACKUP_KEEP` 覆盖）

### 新增
- 新增一键卸载脚本 `scripts/uninstall.sh`，支持 `curl | bash` 执行及 `--purge` 彻底删除模式
- Xray-core 升级至 **v26.3.27**，并支持构建时通过 `ARG XRAY_VERSION` 动态拉取最新版

---

## v1.0.0 - 2026-06-23

- 发布首个稳定版本
- 新增 `scripts/doctor.sh` 部署自检脚本
- 部署验证流程覆盖 Web 健康检查、认证 API、Docker Compose、容器健康及出口镜像

---

## v0.9.0-rc.3 - 2026-06-23

- 面板新增系统状态与备份/还原操作引导

---

## v0.9.0-rc.2 - 2026-06-23

- 新增 `/healthz` 和认证 `/api/system/status` 运维端点
- 为面板和 worker 容器增加 Docker Compose 健康检查
- 新增 `.env` 和持久化数据的备份与还原脚本
- 更新后端依赖，修复 npm audit 安全问题

---

## v0.9.0-rc.1 - 2026-06-23

- 引入 SQLite 持久化与操作任务历史记录
- 将创建、重建、删除等耗时操作拆分至独立 worker 进程
- 面板新增任务队列可视化与任务详情查看
- 新增所有活跃出口的订阅链接输出
- 面板与 API 增加 HTTP Basic Auth 认证
- 新增 `.env.example` 及生产部署默认值配置
- 默认 Reality 混淆域名更新为 `www.amd.com`
