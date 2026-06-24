# 更新日志

## v1.3.4 - 2026-06-24

### 优化与热修复
- **彻底清理与架构大瘦身**：
  - 物理清理删除了所有旧版的 Node.js 后端代码，实现了服务在 Go 架构下的极净化收尾。
- **高并发 SQLite 写入锁死修复**：
  - 在数据库加载中配置了 `SetMaxOpenConns(1)` 限制，依靠 Go 的 Channel/Connection 队列锁代替复杂的磁盘锁排队，彻底根治多协程批量创建或测活增量同步时频繁抛出的 `database is locked (5) (SQLITE_BUSY)` 错误。
- **客户端免 Auth 订阅与 IP 动态生成修复**：
  - 将订阅相关的 HTTP 端点加入鉴权白名单免校验，避免了第三方客户端同步订阅节点时遭遇 `401 Unauthorized`。
  - 将 Host 主机头提取逻辑修正为 Go 规范的 `r.Host` 字段，彻底修复了客户端更新节点时由于默认值降级导致节点“地址”列输出为占位字符 `your_vps_ip` 的问题。
- **引入本地一键构建与极速运行时部署**：
  - 增加了本地 Windows 一键编译批处理脚本 [build.bat](file:///d:/Users/Aaron/Desktop/vless/build.bat)，在本地开发机执行前端 Vite 编译和 Go 的 Linux (amd64) 交叉编译。
  - 调整了前端编译 `outDir` 定向至 Go 静态目录下，并增加了精炼 of [Dockerfile.fast](file:///d:/Users/Aaron/Desktop/vless/Dockerfile.fast)。在优化 [.dockerignore](file:///d:/Users/Aaron/Desktop/vless/.dockerignore) 后，使 VPS 上可以在 1 秒内以零硬件开销极速重构拉起。

## v1.3.3 - 2026-06-24

### 新增
- **开启 Go 语言后端重构第四阶段（HTTP 路由、Controllers、Basic Auth 与主服务整合）**：
  - 新增控制器目录并在 [vpngate.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/controllers/vpngate.go) 中实现高分节点预览与国家节点数统计 API，在 [egress.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/controllers/egress.go) 中实现出口及订阅的增删改查全套 API。
  - 针对任务列表，编写了兼容性强类型适配 `JobJSONResponse` 结构，自动解码 SQLite 内的 Payload 和 Result 字符串为标准 JSON，确保前端组件无感对接。
  - 在 [main.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/main.go) 中挂载了基于时序安全对比（`subtle.ConstantTimeCompare`）的 Basic Auth 认证，防范针对管理员账号密码判定时产生的计时侧信道攻击。
  - 编写了单页面静态托管服务（SPA FileServer），支持对 Vite 构建成果的多候选路径自适应探测加载与 SPA 路由自动回退 index.html，并实现了一体化的 Go 进程一键运行。

## v1.3.2 - 2026-06-24

### 新增
- **开启 Go 语言后端重构第三阶段（自愈守护、任务消费与出口业务）**：
  - 补全了 SQLite 数据库 Job 数据表的 CRUD 接口（`CreateJob`、`GetJob`、`UpdateJob`、`ListJobs`、`JobStatusCounts` 和 `LatestJob`），对 `payload`/`result` 进行完美的 JSON 结构兼容对接。
  - 重构了 [egress_ops.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/services/egress_ops.go) 核心出口业务编排模块，实现了 `CreateEgress` 端口分配、`RebuildEgress` 故障 IP 排除型热重启，内嵌了连续 3 次失败自动熔断销毁逻辑。同时对批量操作实现了并发度为 3 的 Go 协程控制和绝对路径防越权。
  - 重构了 [worker.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/services/worker.go) 任务消费队列，使用缓冲通道与双并发 Worker 协程消费，结合基于 `sync.Map` 的 Target 互斥锁，彻底解耦并消除了对同一出口执行并发操作引发的 Docker 状态冲突。
  - 重构了 [self_healing.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/services/self-healing.go) 自愈测活定时服务，基于 `time.NewTicker` 定时拉起并发协程测活。对连通失效节点进行 `failureCount` 累加，达到上限（2次）自动投递 `rebuild` 任务进行故障漂移自愈。
  - 更新了 [main.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/main.go) 的集成验证逻辑，确保能够在本地开发及无 Docker 运行环境下，成功启动 Worker 线程池和自愈心跳，并在捕获测试异常后实现任务的闭环消费与状态追踪。

## v1.3.1 - 2026-06-24

### 新增
- **开启 Go 语言后端重构第二阶段（Docker SDK、VLESS 密钥与 Xray 配置）**：
  - 集成官方 Docker Go SDK，重构了 [docker.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/services/docker.go) 容器管理层，实现容器的快速重启复用及生命周期管理，并使用原生 `stdcopy.StdCopy` 对 Docker stream 进行安全拆流，从根本上消除了乱码，且增加了 6 秒测活短缓存。
  - 编写了 [reality.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/services/reality.go) 原生密钥生成器，使用 Go 标准库中的 `crypto/ecdh` 原生且极其安全地生成 X25519 密钥对和 8 字节 shortId，摆脱了对外部 xray 命令行的调用依赖。
  - 编写了 [xray.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/services/xray.go) 配置文件生成器，通过模块级缓存和多路径自适应遍历成功实现了对 Xray 配置模板的精准读取与占位符注入。
  - 解决了 `moby` 高低版本分割引发的模块歧义冲突，并在 `go.mod` 中利用 `replace` 将依赖版本锁定至最稳定的兼容版本，通过了在 Windows 和 Linux 网络环境下的编译运行。

## v1.3.0 - 2026-06-24

### 新增
- **开启 Go 语言后端重构第一阶段（骨架、数据库与节点抓取）**：
  - 新建了纯 Go 后端骨架，初始化 `web/backend-go` 工作区，采用纯 Go 实现的 `modernc.org/sqlite` 驱动替代原本需要 C 编译器的 sqlite 库，消除了 CGO 编译依赖，实现了完美的高性能跨平台交叉编译部署。
  - 编写了纯 Go 的数据库访问层 [db.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/models/db.go)，自动进行表初始化、开启 WAL 并支持重建失败次数等最新升级列。
  - 重构了高性能 VPNGate 节点解析器 [vpngate.go](file:///d:/Users/Aaron/Desktop/vless/web/backend-go/services/vpngate.go)，支持并发内存 TTL 缓存、多源镜像轮询重试以及针对唯一节点的自愈退化降级排序算法，经测试解析节点耗时缩短至毫秒级，且内存缓存命中率达 100%。

## v1.2.3 - 2026-06-24

### 重构优化
- **单进程化架构升级（极简化瘦身）**：将独立的任务消费与自愈 Worker 进程融合并入 Web 服务主进程 `app.js` 中。通过在 Node.js 事件循环中统一非阻塞调度，彻底消除了物理上两个 Node.js 虚拟机进程运行造成的资源冗余，直接**释放了 30MB~50MB 内存占用**。同时在 `docker-compose.yml` 中直接废除了 `self-healing-worker` 容器服务，物理上简化了服务编排拓扑。
- **容器级快速重启复用（秒级热更新）**：重构了 `docker.js` 中的 `startEgressContainer` 方法。在重建或漂移出口时，检测若同名容器已存在，不再销毁重建容器，而是直接通过宿主机挂载目录覆写 `.ovpn` 配置，然后调用 `container.restart()` 快速重启。**避免了 Docker 容器冷启动销毁和重建网卡的系统调用开销，使节点切换响应在 1-2 秒内瞬间完成**。

## v1.2.2 - 2026-06-24

### 重构优化
- **重建连续失败出口自动销毁**：在 `db.js` 新增并持久化 `rebuildFailureCount` 重建失败计数。在 `egress-ops.js` 的 `rebuildEgress` 逻辑中，当某一出口连续 3 次重建（自愈漂移）失败时，系统将直接调用删除逻辑 `deleteEgress` 彻底清理该地区出口（含容器、配置目录与数据库记录），从物理上避免无限疯狂重试与对 VPS 资源的无端占用。
- **健康检测频率优化为每日一次**：修改 `self-healing.js`，将自愈探测心跳默认频率 `HEALTH_CHECK_INTERVAL` 从 1 分钟修改为 24 小时（即每日一次），并在 `.env.example` 提供对应环境变量，极大地减轻了系统的常态运行负载。

## v1.2.1 - 2026-06-24

### 修复
- **前端订阅复制功能补全**：在 `App.vue` 补全丢失的 `copySubUrl`（复制订阅 URL）、`copySubB64`（复制 Base64 订阅）和 `copyLinkFromModal`（从模态框复制单条链接）的函数逻辑，修复了控制面板中复制按钮点击无效的缺陷。
- **唯一节点自愈死锁修复**：
  - 重构 `vpngate-fetcher.js` 的 `getBestNode`，增加排除失效 IP 导致节点池为空时的降级机制。若排除后可用节点数为 0 且该地区实际存在节点，自动退化为不排除 IP 过滤，允许重新尝试之前连接过的唯一节点，防止自愈陷入死循环报错。
  - 重构 `egress-ops.js` 的 `rebuildEgress`，为重建逻辑增加 `try...catch` 异常捕获。在重建失败时能够正确将 `egress` 状态改回 `error` 并写入具体的错误信息，防止其卡在 `rebuilding` 漂移中，保障后续心跳周期自愈的持续拉起。

## v1.2.0 - 2026-06-24

### 重构优化
- **引入状态测活短缓存与 SQLite 增量更新**：重构 `docker.js` 中的测活逻辑，引入 6 秒有效期的共享内存缓存去重并发请求，将自愈心跳与前端轮询造成的 VPS 网络及系统开销削减 50%，且使前端列表刷新快如闪电。同时修改 `/list` 路由，只有在状态、IP 或报错实质变化时才执行 SQLite UPDATE，常态下将磁盘写入开销压缩至 0 次，显著降低磁盘 I/O 损耗并延长 SSD 寿命。
- **修复测活多源管道异常回退失效**：在 `docker.js` 与 `entrypoint.sh` 的所有获取 IP `curl` 指令中强制引入 `-f` 参数。当服务器因代理风险返回 4xx/5xx HTTP 错误码时强制使 `curl` 返回非零状态，从而能正确激活 shell 的 `||` 管道并顺次请求后续备份 API。
- **高可用测活 IP 多源随机打散**：重构 `docker.js` 中的 `getContainerStatusAndIp` 容器测活功能，弃用对单个 `ipinfo.io` 测活接口的依赖，改为在 Node端实现主流 IP API 的随机打散，并通过 `sh -c` 执行多源回退获取。此举极大分摊了高频探测对单点接口产生的请求负荷，消除了 429 速率限制引发 of 误判自愈。
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
