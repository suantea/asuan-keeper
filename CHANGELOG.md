# asuan-keeper 更新记录

> 本文件按时间倒序记录每次发布/功能变更，便于后续检查与回溯。
> 发布触发：`git tag vX.Y.Z && git push origin vX.Y.Z` → GitHub Actions 自动构建+发布。

## 2026-08-30（未发版）

- **build: 占位符虚拟层拆分为 `-tags cgofuse` 可选构建 + 新增三平台 CI 测试**
  - `internal/placeholder/fs.go`（cgofuse/FUSE 实现）挂 `cgofuse` build tag，新增 `fs_nofuse.go` 桩：
    无 FUSE 头文件的机器（如未装 macFUSE 的 macOS）上 `go build ./...` / `go test ./...`
    不再失败，release/hydrate/列表/缓存等非挂载功能照常编译测试。
  - 发行构建（`scripts/build-dist.sh`、hub Dockerfile）统一加 `-tags cgofuse`，包含真实虚拟层。
  - 新增 `.github/workflows/test.yml`：ubuntu/macos/windows 三平台 push/PR 跑构建与测试
    （此前只有 tag 触发的 release 构建，测试从未在 CI 跑过）。
- **feat(syncthing): 引擎更新完整性校验 + 下载加固**
  - `asuan engine-update` 新增 `--sha256`：下载包按官方 SHA256SUMS 校验，失败不触碰现有引擎；
    `ASUAN_ENGINE_BASE` 镜像本身不可信，此前下载后无任何校验即替换二进制。
  - `downloadFile` 弃用裸 `http.Get`：加超时客户端与 512MB 大小上限，异常响应不再可能写满磁盘。
- **fix(placeholder): `.stignore` 读改写加锁 + FUSE Read 的 EOF 判断修正**
  - `AddRules`/`RemoveRules` 包内互斥锁：控制台 hydrate-many 并发 3 路同时改规则会互相覆盖；
    （跨进程并发仍需使用者自行避免。）
  - `fs.go` 的 EOF 判断从字符串比较改为 `errors.Is(err, io.EOF)`。
- **docs: README 重写**——补「它是怎么工作的」架构说明（编排器 + sidecar + `(?d)` 释放机制）、
  澄清单一二进制（NAS hub = 同一二进制的 Docker 化运行，此前"两个入口"的说法已过时）、
  新增开发与测试章节（构建 tag、CI 说明）、`engine-update --sha256` 用法。

## 2026-08-17（未发版，已推 AtomGit main）

- **fix(syncthing): 相对 bin 路径解析为绝对路径（修复 "cannot run executable found relative to current directory"）**
  - 此前 `syncthing.bin` 为相对名（如 rename 产生的 `syncw.exe`）时，Go 1.19+ 的 exec.Command 拒绝执行不带 `./` 前缀的相对路径可执行文件，导致 syncthing 启动失败、asuan 立即退出、网页控制台无法访问。现在 New() 会把相对 bin 在 exeDir 下解析为绝对路径。
- **feat(deploy): 无窗口后台启动器「后台启动-asuan.vbs」**
  - 双击即用 wscript 隐藏窗口后台运行 asuan（无控制台黑窗、无需常开窗口），停止走托盘或 `asuan stop`；build-dist.sh 打包附带，README-QUICKSTART 同步说明。
- **feat(perf): sidecar 低优先级**
  - 新增 internal/procprio：Windows 启动 syncthing 时设 Below Normal 调度优先级（OpenProcess+SetPriorityClass），避免抢占前台应用；其他平台 no-op。
- **docs(ENGINE.md): config.xml 敏感字段收敛 + 协议指纹缓解**
  - 新增「config.xml 敏感字段收敛」（GUI API Key 明文/0600 权限/数据目录改名指引）与「协议指纹与缓解」（证书/BEP 指纹 + 端口自定义/连接白名单/设备白名单/发现全关组合）。
- **feat(efficiency): 压缩策略可配 + 日志瘦身 + 同步监控增强**
  - 传输压缩：config 新增 `syncthing.compression`（metadata 默认 / full 适合 WAN / off 局域网最快），applyDevices 统一应用到所有对端；表单「基本信息」可选。
  - 日志瘦身：`syncthing.log_max_mb`（默认 5MB），超限自动截断为 .1（syncthing 2.1.3 无 logLevel 字段，实测确认后改走大小轮转）。
  - 同步监控：状态页新增 ↓ 接收 / ↑ 发送速率卡片（web 端两次采样差值 B/s）；文件夹错误摘要（db/status error 字段）随状态返回，异常时顶部徽章标红。
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
- **文档**：MAC.md / hub README / asuan.example.json。

> 勘误：本文件旧版 2026-08-14 条目曾列出「hub 配置中心 `GET /v1/sync-config` + `asuan sync-config`」
> 与「QNAP 部署教程（deploy/QNAP-deploy.md）」——经核对代码库中均不存在（未合并或已回退），已移除。

## 更早记录

- **P0 基础同步**：init / run / status / stop、网页控制台（:18084）、NAS hub（Docker）、占位符、引擎隐身化、系统托盘、防火墙规则管理。
- **发布流水线（GitHub Actions）**：`v*` tag 或手动触发 → windows-amd64/arm64 + linux-amd64 → GitHub Releases → Docker Hub `suantea/asuan-keeper:latest` + `:vX.Y.Z`（已验证 v2.0.2）。
- **Docker 镜像**：`suantea/asuan-keeper`，含 deploy/hub/docker-compose*.yaml（本地构建/完全版/精简版）。

## 已知待办

- 自动释放策略：磁盘剩余水位 / 文件年龄触发占位符 auto-release（blocks: release/hydrate 原语已就绪）。
- 虚拟层水合去重：并发打开同一文件合并为一次拉取；水合等待中加入文件大小一致性校验（syncthing 落盘经临时文件，存在读到半截内容的窗口）。
- `.stignore` 跨进程互斥（CLI 与控制台同时操作同一文件夹）。
- 引擎校验和清单内置（当前 `--sha256` 为手动提供）。
- QNAP 部署收尾与实机验证清单（见 README）逐项勾选。
