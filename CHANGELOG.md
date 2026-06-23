# 更新日志

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
