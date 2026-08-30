# asuan-keeper

> 文件缓存与分发工具：局域网优先同步，NAS 常驻中转，占位符按需拉取释放本地空间。

## 它是怎么工作的

asuan 是一个围绕 [Syncthing](https://syncthing.net/) sidecar 引擎的**编排器**，加上一层**占位符（placeholder）能力**：

```
┌──────────────┐  REST(/rest/*)  ┌─────────────┐   BEP/TLS    ┌──────────────┐
│ asuan        │ ──────────────▶ │ syncthing   │ ◀──────────▶ │ 对端 syncthing│
│ (编排/托盘/   │   管理配置/扫描   │ (sidecar,   │  静态 peer    │  (NAS hub /   │
│  控制台/FUSE) │                 │  隐身化配置)  │  端口 44312   │  其他客户端)   │
└──────────────┘                 └─────────────┘              └──────────────┘
```

- **同步内核**：本地起一个 `syncthing` 子进程（同目录自动识别），关闭发现/中继/NAT/UPnP、
  自定义端口、改名去主机名——即"引擎隐身化"；对端是静态 `tcp://IP:44312`，节点间认证靠
  Syncthing 设备 ID（BEP/TLS），没有账号体系。
- **释放（release）/ 水合（hydrate）**：这是与"裸 Syncthing"的核心差异。
  - *释放*：往文件夹的 `.stignore` 写 `(?d)` 规则（文件夹级 `(?d)*` 或单路径 `(?d)/path`），
    再删除本地实体——"忽略删除"让删除**不传播**，对端内容保留，本地空间即刻释放。
  - *水合*：移除对应规则并重扫，内容从对端重新拉回；挂载了虚拟层时，打开占位文件即自动触发水合。
- **占位符虚拟层**（可选，`-tags cgofuse` 构建）：基于 cgofuse 的只读 FUSE 文件系统。
  已释放文件夹挂载后，对端文件以"占位条目"可见（名字/大小来自对端索引），双击打开即自动
  水合并从本地真实路径读取——近似 OneDrive"按需文件"体验。
- **网页控制台**（:18084）：状态总览 / 配置表单 / 批量释放与水合（并发 3），可选访问令牌。

## ✨ 功能特性

- 局域网优先同步，NAS 作为常驻中转节点（全量内容 + 30 天回收站）
- 占位符释放/水合：文件夹级与单文件/单目录级，本地空间按需回收
- 占位符虚拟层：挂载后"双击水合"（Windows/WinFsp · macOS/macFUSE · Linux/FUSE）
- 引擎隐身化：消除 sidecar 引擎联网特征（关发现/中继/NAT/UPnP、自定义端口、改设备名）
- 连接白名单：`stealth.allowed_networks` 走系统防火墙 remoteip，设备列表收敛为本机 + 已声明 peers
- Windows 体验：托盘常驻、开机自启（HKCU）、防火墙规则管理（`asuan firewall`）、引擎改名（`asuan rename`）
- 引擎更新：`asuan engine-update`（可选 `--sha256` 校验下载包完整性），发行包随附引擎
- 多端：Windows / macOS / NAS (Docker)，客户端绿色免安装

## 快速开始

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

常用命令一览：

```
asuan init|run|status|stop|config      基础生命周期
asuan release <folder> [relpath]       释放（删本地实体不传播，对端保留）
asuan hydrate <folder> [relpath]       水合（移除规则，从对端拉回）
asuan engine|engine-update             引擎版本说明 / 更新（--sha256 可选校验）
asuan firewall <status|add|remove>     Windows 防火墙规则管理
asuan autostart <status|on|off>        开机自启（Windows，HKCU）
asuan rename <新名字>                   引擎二进制改名（隐蔽进程名）
```

设备交换：每端 `asuan status` 显示的设备 ID 填入其他端的 `peers`。

> 引擎版本对照与更新：发行包内置的引擎版本见 `asuan engine`；升级由 asuan 统一管理
> （`asuan engine-update [--sha256 <校验和>]`），小版本换二进制即可，无需改 asuan 自身。

## 文件布局（配置与引擎分离）

程序目录（绿色便携，asuan 可执行文件所在处）：

```
asuan(.exe)      # 本工具
syncthing(.exe)  # 同步引擎（可选，同目录自动识别）
asuan.json       # asuan 自身配置（默认就在程序同目录，不与其他配置混用）
syncthing/       # 引擎自己的 config.xml 与数据（子目录，与 asuan.json 分开）
```

`asuan.json` 只属于 asuan，引擎内部配置始终放在程序目录的 `syncthing/` 子目录，两者互不通用。

## 引擎特征与更新

asuan 自动消除 sidecar 引擎的联网特征（写入引擎配置）：

- 关闭全局/本地发现、中继、NAT、UPnP（默认配置即全部关闭）
- 监听端口自定义为 `stealth.tcp_port`，不暴露引擎默认端口
- 本机设备名改为 `asuan.json` 的 `name`，不泄露主机名
- 关闭使用统计上报、自动升级、开机自启、自动打开浏览器

`asuan engine-update` 默认从 GitHub Releases 下载（`ASUAN_ENGINE_BASE` 可指向镜像），
下载带超时与大小上限；传 `--sha256`（取自官方 SHA256SUMS）时强制校验完整性，校验失败不触碰现有引擎。
详见 [deploy/ENGINE.md](deploy/ENGINE.md)。

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

## 开发与测试

```
cmd/asuan/            唯一入口：NAS hub 即同一二进制的 Docker 化运行
internal/syncthing/   引擎编排（隐身化、设备/文件夹收敛、版本与更新、校验）
internal/placeholder/ 释放/水合（.stignore (?d) 规则）、虚拟层（FUSE）、目录缓存
internal/web/         内嵌网页控制台（:18084）
internal/config/      asuan.json（portable，随二进制）
internal/firewall|autostart|procprio|tray   平台集成
deploy/               hub Docker 编排、ENGINE.md、MAC.md、启动器
```

```bash
# 无需 FUSE 头文件即可完整构建/测试（占位符虚拟层走 stub，fs_nofuse.go）
go build ./...
go test ./...

# 构建带真实虚拟层的二进制（需要 WinFsp / macFUSE / FUSE 头）
go build -tags cgofuse ./cmd/asuan

# 发行包（自动下载引擎 + 打包 zip）
bash scripts/build-dist.sh [windows|darwin|linux] [amd64|arm64]
```

CI：`.github/workflows/test.yml` 在 ubuntu/macos/windows 三个平台跑 `go build ./...` + `go test ./...`（stub 模式，保证干净环境全绿）；`.github/workflows/release.yml` 在 `v*` tag 时以 `-tags cgofuse` 构建发行包并推 Docker Hub。

## 实机验证清单

开发环境仅验证到"编译 + 单机运行 + API"层面；以下项需在真实机器上逐条确认（勾选即通过）：

- **Windows 客户端**：程序目录放 `asuan.exe` + `syncthing.exe`，`asuan init` 后编辑 `asuan.json`，`asuan run` 启动
- **Windows 托盘交互**：启动后最小化到托盘；左键单击弹出网页控制台（进度），左键双击打开配置，右键菜单可退出/暂停-同步
- **Windows 防火墙**：管理员执行 `asuan firewall add` 后重启 `asuan run`，确认不再弹"允许访问"对话框
- **占位符（Windows/WinFsp）**：安装 WinFsp，配置 `placeholder.mount` 后释放文件夹，虚拟层可见对端文件、访问触发水合
- **Win ↔ NAS 双端联调**：按 `deploy/hub/README.md` 部署 hub（含防火墙放行），两端互填设备 ID，文件双向同步、删除进回收站
- **macOS 客户端**：按 `deploy/MAC.md` 安装 macFUSE、以 `-tags cgofuse` 本机构建，托盘与占位符（macFUSE 挂载/水合）实机验证
- **远程限速**：配置 `remote` 段 + WireGuard 隧道后，远程连接按 `limit_kbps` 限速、LAN 直连满速

## 后续候选

- 自动释放策略（磁盘剩余水位 / 文件年龄触发的占位符 auto-release）
- 虚拟层水合去重（并发打开同一文件合并为一次拉取）与水合完成前的文件大小校验
- 引擎校验和清单内置（当前 `--sha256` 为手动提供）
- 安装包分发

## 许可与声明

- asuan 自身代码采用 Mozilla Public License 2.0（MPL-2.0）授权。
- 本项目运行时调用第三方组件 **Syncthing**（MPL-2.0，<https://syncthing.net/>），并以**二进制随发行包内置**（sidecar 进程方式调用，未修改其源代码）。按 MPL-2.0 分发要求，发行包随附官方包内 `LICENSE.txt`、`AUTHORS.txt`、`README.txt` 及本项目 [NOTICE](NOTICE)，声明其来源与许可证；相关详情见 [NOTICE](NOTICE) 与 [deploy/ENGINE.md](deploy/ENGINE.md)。
- 引擎版本对照与更新方式见 `asuan engine`（agent 更新说明）。
