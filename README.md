# asuan-keeper

文件缓存与分发工具。

- 局域网优先同步，NAS 作为常驻中转节点
- 支持占位符（按需拉取），释放本地空间
- 多端：Windows / macOS / NAS (Docker)
- 客户端绿色免安装，占用极低

## 快速开始（P0：基础同步）

端上程序目录放 `asuan.exe` + `syncthing.exe`，同目录建 `asuan.json`（可先 `asuan init` 生成）。

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

## NAS hub（Docker）

见 `deploy/hub/README.md`。NAS 常驻持有全量内容 + 回收站（保留 30 天）。

## 许可与声明

- asuan 自身代码采用 Mozilla Public License 2.0（MPL-2.0）授权。
- 本项目运行时会调用第三方组件 **Syncthing**（MPL-2.0，<https://syncthing.net/>）。Syncthing 以独立 sidecar 进程方式被调用，未修改其源代码；相关来源、许可证与分发要求见 [NOTICE](NOTICE)。
- 引擎版本对照与更新方式见 `asuan engine`（agent 更新说明）。

## 组件

| 组件 | 说明 |
|------|------|
| `asuan` | 端上守护进程（Win/Mac/Linux），管理同步引擎与占位符 |
| `asuan-hub` | NAS 常驻节点（Docker），持有全量内容与回收站 |

