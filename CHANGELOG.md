# asuan-keeper 更新记录

> 本文件按时间倒序记录每次发布/功能变更，便于后续检查与回溯。
> 发布触发：`git tag vX.Y.Z && git push origin vX.Y.Z` → GitHub Actions 自动构建+发布。

## 2026-08-17（未发版，已推 AtomGit main）

- **feat(web): 控制台可选访问令牌（web.token）**
  - config 新增 `web.token`（空=不鉴权，默认关）；设置后除页面本身外所有 /api/* 请求需携带 `X-Auth-Token` 头（或 `?token=`），否则 401。
  - 前端 fetch 自动携带 token（存 localStorage，401 时提示输入）；表单「基本信息」新增令牌输入框。
- **feat(stealth): 引擎二进制改名（asuan rename）**
  - 新增 `asuan rename <新名字>`：引擎二进制改名（自动补 .exe，旧文件保留 .bak 可回退），更新 asuan.json 的 `syncthing.bin` 指向新名——任务管理器/进程列表不再显示 syncthing。
  - 名字白名单校验（字母/数字/下划线/连字符，防路径注入）。
- **feat(autostart): 开机自启（免管理员）**
  - 新增 `asuan autostart <status|on|off>`：HKCU Run 注册表（无需管理员）+ wscript 隐藏窗口启动器（登录后无控制台黑窗自动后台启动 run）。
- **feat(placeholder): 占位符驱动检测与启用引导**
  - 新增 `DriverAvailable()`（Windows 查 winfsp.sys / macOS macFUSE / Linux /dev/fuse）；配置了 `placeholder.mount` 但缺驱动时给出明确安装引导（WinFsp/macFUSE/FUSE 下载地址），不再静默失败。
  - 控制台表单新增「占位符虚拟层」卡片（挂载点输入 + 说明）。
- **feat(web): 批量释放/水合 + 并发上限**
  - 新增 `/api/release-many`、`/api/hydrate-many`（folder 数组，并发 3 上限，逐项汇总结果）；状态总览「同步文件夹」新增「释放全部/水合全部」按钮。
- **feat(stealth): 连接白名单（allowed_networks）**
  - config 新增 `stealth.allowed_networks`（CIDR 或 IP 列表，空=不限制）；控制台表单「网络与隐蔽」可编辑。
  - **网络层白名单走系统防火墙 remoteip**（`firewall add` 生成带 `remoteip=` 的入站规则，仅允许白名单网段访问同步端口；⚠️ 实测 syncthing 2.1.3 options **无 allowedNetworks 字段**，PUT 静默忽略，故不走 syncthing）。
  - **设备级白名单**：applyDevices 收敛 devices 列表为「本机 + 已声明 peers」，清除 syncthing 自动发现/历史残留设备，减少 BEP 暴露面。
- **feat(web): 控制台 UI 重构——状态总览/配置双页签 + 视觉美化**（`30ebfe5`）
  - 页面改为「状态总览 / 配置」双页签：状态卡片+文件夹+对端表格与配置表单分离，避免超长滚动、逻辑更清晰。
  - 顶栏加 logo/运行徽章/设备 ID；分区标题带色条与说明；卡片圆角阴影、表格悬停、统一按钮样式。
  - 配置表单卡片分组（基本信息 / 网络与隐蔽 / 远程访问 / peers / folders / 高级项）；空状态提示指向配置页。
- **feat(web,firewall): 网页配置表单 UI + 无需管理员权限优化**（`650ebf1`）
  - 控制台配置区新增「表单编辑」模式（默认）：基本信息 / stealth 开关 / 远程访问 / **peers 表格增删行** / **folders 表格增删行**（policy、versioning trashcan·staggered、保留天数）；保留「JSON 编辑」切换；保存走 `/api/config` 校验+保存+重载。
  - firewall 新增 `IsAdmin()`（Windows 用 `token.IsElevated()`，规避 UAC deny-only 组导致的 IsMember 误判）；非管理员执行 `firewall add/remove` 输出友好降级提示，不再直接报错。
  - 新增 Windows 双击启动器 `deploy/启动-asuan.bat`（自动 init + run + 打开控制台），build-dist.sh 打包时附带（UTF-8 源码转 GBK+CRLF）。
  - README-QUICKSTART 更新：双击启动器用法 + 分字段表单说明 + 无需管理员声明 + 防火墙弹窗规避。

## 2026-08-14（已发版 / 已推 AtomGit main）

- **feat(config): 默认同步端口固定 44312，文件夹支持自定义 versioning/冲突副本数**（`ad7e122`）
  - `asuan init` 各端开箱一致使用 44312；folders 支持自定义 versioning（回收站/分层）与 max_conflicts。
- **feat(placeholder): 占位符目录列表增加 TTL 缓存**（`196e702`）
  - 大目录浏览不再反复打 syncthing REST API，浏览延迟显著下降。
- **hub 配置中心**（`daaf30d` / `c1db38f` / `8f188ad`）
  - `GET /v1/sync-config`（Bearer token 鉴权）+ 客户端 `asuan sync-config` 一键拉取合并网络配置。
- **文档**：MAC.md / hub README / asuan.example.json / QNAP 部署教程（deploy/QNAP-deploy.md）。

## 更早记录

- **P0 基础同步**：init / run / status / stop、网页控制台（:18084）、NAS hub（Docker）、占位符、引擎隐身化、系统托盘、防火墙规则管理。
- **发布流水线（GitHub Actions）**：`v*` tag 或手动触发 → windows-amd64/arm64 + linux-amd64 → GitHub Releases → Docker Hub `suantea/asuan-keeper:latest` + `:vX.Y.Z`（已验证 v2.0.2）。
- **Docker 镜像**：`suantea/asuan-keeper`，含 deploy/hub/docker-compose*.yaml（本地构建/完全版/精简版）。

## 已知待办（详见项目进度笔记）

- QNAP 部署收尾：重建镜像（托盘修复 + debian-slim 运行基础）→ 上传 → 重建容器 → 验证 18084 可达。
- 连接白名单（peers + allowedNetworks）、Web 控制台可选 token、开机自启、占位符虚拟层（WinFsp）落地。
