# asuan-keeper

> 文件缓存与分发工具：局域网优先同步，NAS 常驻中转，占位符按需拉取释放本地空间。

## ✨ 功能特性

- 局域网优先同步，NAS 作为常驻中转节点
- 支持占位符（按需拉取），释放本地空间
- 多端：Windows / macOS / NAS (Docker)
- 客户端绿色免安装，占用极低
- 引擎隐身化：自动消除 sidecar 引擎的联网特征，不暴露默认端口与主机名

## 快速开始（P0：基础同步）

**发行包开箱即用**：`asuan` 与同步引擎 `syncthing` 已一并打包进发行包（绿色便携，无需单独下载/安装引擎）。解压后程序目录为 `asuan.exe` + `syncthing.exe`，同目录建 `asuan.json`（可先 `asuan init` 生成）。

1. **init**：生成默认配置并初始化同步引擎，输出本机设备 ID
   ```
   asuan init
   ```
2. **配置 `asuan.json`**：
   - `name`：本机名字
   - `peers`：对端设备 ID + 局域网静态地址（`tcp://<IP>:<端口>`）
   - `folders`：要同步的文件夹（`id` 各端一致、`path` 各自本机路径、`policy` 为 `sync`）
3. **运行**：`asuan run`（前台）查看 `asuan status`
4. **网页控制台**：运行后浏览器打开 `http://127.0.0.1:18084`（hub 为 `http://<NAS>:18084`），可看同步状态、改配置
5. **停止**：`asuan stop`

设备交换：每端 `asuan status` 显示的设备 ID 填入其他端的 `peers`。

> 引擎版本对照与更新：发行包内置的引擎版本见 `asuan engine`；升级由 agent 统一管理（`asuan engine-update`），小版本换二进制即可，无需改 asuan 自身。

## 文件布局（配置与引擎分离）

程序目录（绿色便携，asuan.exe 所在处）：

```
asuan.exe        # 本工具
syncthing.exe    # 同步引擎（可选，同目录自动识别）
asuan.json       # asuan 自身配置（默认就在程序同目录，不与其他配置混用）
syncthing/       # 引擎自己的 config.xml 与数据（子目录，与 asuan.json 分开）
```

`asuan.json` 只属于 asuan，引擎内部配置始终放在程序目录的 `syncthing/` 子目录，两者互不通用。

## 引擎特征与更新（agent 对照）

asuan 自动消除 sidecar 引擎的联网特征（写入引擎配置）：

- 关闭全局/本地发现、中继、NAT、UPnP（默认配置即全部关闭）
- 监听端口自定义为 `stealth.tcp_port`，不暴露引擎默认端口
- 本机设备名改为 `asuan.json` 的 `name`，不泄露主机名
- 关闭使用统计上报、自动升级、开机自启、自动打开浏览器

引擎版本对照与更新由 asuan agent 管理，`asuan engine` 查看说明，`asuan engine-update` 自动更新（可 `ASUAN_ENGINE_BASE` 指定下载镜像）。详见 [deploy/ENGINE.md](deploy/ENGINE.md)。

## 系统防火墙（Windows）

sidecar 首次监听端口时，Windows 防火墙可能弹出"允许访问"对话框，暴露进程与端口，违背隐蔽设计。为此 asuan 提供防火墙规则管理：

```bash
asuan firewall status    # 查看 stealth.tcp_port 是否已放行
asuan firewall add       # 预置入站放行规则（需管理员）
asuan firewall remove    # 移除规则
```

- 规则名中性（`asuan-sync-<端口>`），按端口而非程序路径放行，不暴露 syncthing 位置
- `stealth.tcp_port` 为 0（随机端口）时无法预置，需先在 `asuan.json` 固定端口
- 首次部署执行一次 `asuan firewall add` 后不再弹窗

## NAS hub（Docker）

NAS 常驻持有全量内容 + 回收站（保留 30 天）。镜像已发布到 Docker Hub（`suantea/asuan-keeper`），推荐直接拉取镜像部署，无需本地构建：

- **快速部署（推荐）**：完整部署文档见 [deploy/hub/README.md](deploy/hub/README.md)，含精简版 / 完全版 docker-compose
- **精简版 compose**：[deploy/hub/docker-compose.minimal.yaml](deploy/hub/docker-compose.minimal.yaml)（复制即用）
- **完全版 compose**：[deploy/hub/docker-compose.remote.yaml](deploy/hub/docker-compose.remote.yaml)（含逐项注释）
- **本地源码构建版**：[deploy/hub/docker-compose.yml](deploy/hub/docker-compose.yml)（`build: .`，不上镜像仓库）

快速上手：

```bash
# 1. 建目录、放配置（复制 deploy/hub/asuan.example.json 修改 peers/folders/tcp_port）
mkdir -p asuan-keeper && cd asuan-keeper
cp <本仓库>/deploy/hub/asuan.example.json asuan.json

# 2. 用精简版或完全版 compose 启动（自动从 Docker Hub 拉取 suantea/asuan-keeper 镜像）
docker compose up -d

# 3. 打开网页控制台
#    http://<NAS>:18084
```

镜像基于官方 `syncthing/syncthing:2.1.3`（MPL-2.0），详见 [NOTICE](NOTICE) 与 [deploy/hub/README.md](deploy/hub/README.md)。

## 目录结构

```
cmd/           入口（端上程序 asuan 与 NAS 节点 asuan-keeper）
internal/      内部 Go 包
deploy/        部署相关（hub/ Docker 编排、ENGINE.md 引擎版本说明）
go.mod         Go 依赖
NOTICE         Syncthing 等第三方声明
```

## 开发预期

- **设计哲学**：局域网优先、占位符按需拉取、绿色便携、引擎隐身（防联网特征暴露）
- **已实现**：P0 基础同步（init / run / status / stop）、网页控制台、NAS hub、占位符、引擎隐身化配置、系统托盘、防火墙规则管理
- **后续候选**（按需）：占位符策略优化、同步状态监控增强、安装包分发

## 实机验证清单

开发环境仅验证到"编译 + 单机运行 + API"层面；以下项需在真实机器上逐条确认（勾选即通过）：

- [ ] **Windows 客户端**：程序目录放 `asuan.exe` + `syncthing.exe`，`asuan init` 后编辑 `asuan.json`，`asuan run` 启动
- [ ] **Windows 托盘交互**：启动后最小化到托盘；左键单击弹出网页控制台（进度），左键双击打开配置，右键菜单可退出/暂停-同步
- [ ] **Windows 防火墙**：管理员执行 `asuan firewall add` 后重启 `asuan run`，确认不再弹"允许访问"对话框
- [ ] **占位符（Windows/WinFsp）**：安装 WinFsp，配置 `placeholder.mount` 后释放文件夹，虚拟层可见对端文件、访问触发水合
- [ ] **Win ↔ NAS 双端联调**：按 `deploy/hub/README.md` 部署 hub（含防火墙放行），两端互填设备 ID，文件双向同步、删除进回收站
- [ ] **macOS 客户端**：按 `deploy/MAC.md` 安装 macFUSE、本机构建，托盘与占位符（macFUSE 挂载/水合）实机验证
- [ ] **远程限速**：配置 `remote` 段 + WireGuard 隧道后，远程连接按 `limit_kbps` 限速、LAN 直连满速

## 许可与声明

- asuan 自身代码采用 Mozilla Public License 2.0（MPL-2.0）授权。
- 本项目运行时调用第三方组件 **Syncthing**（MPL-2.0，<https://syncthing.net/>），并以**二进制随发行包内置**（sidecar 进程方式调用，未修改其源代码）。按 MPL-2.0 分发要求，发行包随附官方包内 `LICENSE.txt`、`AUTHORS.txt`、`README.txt` 及本项目 [NOTICE](NOTICE)，声明其来源与许可证；相关详情见 [NOTICE](NOTICE) 与 [deploy/ENGINE.md](deploy/ENGINE.md)。
- 引擎版本对照与更新方式见 `asuan engine`（agent 更新说明）。

## 组件

| 组件 | 说明 |
|------|------|
| `asuan` | 端上守护进程（Win/Mac/Linux），管理同步引擎与占位符 |
| `asuan-keeper` | NAS 常驻节点（Docker），持有全量内容与回收站 |
