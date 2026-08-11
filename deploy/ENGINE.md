# 引擎（syncthing）特征清除与 agent 更新说明

asuan 将 Syncthing 作为 sidecar 同步内核调用。本文档说明：已清除哪些联网特征、版本如何对照、agent 如何更新。

## 一、特征清除清单

asuan 启动时通过 REST API 覆盖引擎配置，以下特征默认全部清除/关闭（`asuan.json` 的 `stealth` 段控制）：

| 特征 | 默认值 | 说明 |
|---|---|---|
| 全局发现 `globalAnnounceEnabled` | false | 不向公网发现服务器上报 |
| 本地发现 `localAnnounceEnabled` | false | 不在局域网广播 |
| 中继 `relaysEnabled` | false | 不连接公网中继 |
| NAT `natEnabled` / UPnP `upnpEnabled` | false | 不自动做端口映射 |
| 监听端口 | `stealth.tcp_port` 自定义 | 不暴露默认 22000 |
| 使用统计上报 `urAccepted` | -1 | 拒绝 |
| 自动升级 | 0 / STNOUPGRADE | 由 agent 统一管理 |
| 自动打开浏览器 | false | — |
| 本机设备名 | 取 `asuan.json` 的 `name` | 不泄露主机名 |
| 数据目录名 | 程序目录下 `syncthing/` | 可改名（改 `syncthing.data_dir`） |
| GUI 管理界面 | 绑定 `127.0.0.1` 仅本机 | 不对外暴露 |

部署时的额外建议：

- 引擎二进制可改名（`syncthing.bin` 指向任意文件名），数据目录同理。
- 局域网同步端口（`stealth.tcp_port`）在 NAS 防火墙上只放行给内网。
- 进程优先级低、不开官方 GUI，统一走 asuan 网页控制台。

## 二、版本对照（agent 视图）

引擎是独立二进制，asuan 仅依赖其 REST API 与配置格式，因此：

- **小版本升级**（v2.1.3 → v2.1.x）：一般无需改动 asuan，直接替换二进制。
- **大版本升级**：需先核对 `internal/syncthing/version.go` 中 `VerifiedVersions`，并用 `asuan run` + `asuan status` 冒烟。

当前已验证版本：见 `asuan engine` 输出。

## 三、agent 更新机制

```bash
asuan engine            # 查看已安装/推荐/已验证版本对照
asuan engine-update     # 更新到推荐版本（下载→替换→保留 .bak 备份）
asuan engine-update v2.2.0   # 更新到指定版本
```

下载源默认 GitHub releases；网络受限时指定镜像：

```bash
ASUAN_ENGINE_BASE=https://ghproxy.net/https://github.com/syncthing/syncthing/releases/download \
  asuan engine-update
```

更新完成后：若同步正在运行，`asuan stop && asuan run` 重启；再 `asuan status` 确认版本生效。

## 四、开源合规

Syncthing 采用 MPL-2.0。本仓库 [NOTICE](../NOTICE) 记录其来源与许可证；若发行物自带
Syncthing 二进制，须一并携带官方包内 LICENSE.txt / AUTHORS.txt / README.txt。
