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

## 四、config.xml 敏感字段收敛

引擎配置 `syncthing/config.xml` 内含 **GUI API Key（明文）**、设备 ID、证书等敏感字段：

- **文件权限**：生成后即 `0600`（仅属主可读写），请勿手动改宽；如误改，`chmod 600` 恢复。
- **GUI API Key**：是引擎管理界面（`127.0.0.1:8384`）的访问凭据。asuan 已将其绑定 loopback 仅本机；请勿复制到其他机器或提交到仓库。
- **证书/设备 ID**：设备 ID 即公钥指纹，本就在对端交换范围内；证书私钥（`key.pem`）属主权限必须保持 `0600`。
- **数据目录改名**：如需隐藏目录名，改 `asuan.json` 的 `syncthing.data_dir`（如 `syncdata`），重启后自动重建；旧数据目录需自行迁移。

## 五、协议指纹与缓解

syncthing 的 TLS 自签名证书与 BEP 协议存在可识别指纹，主动扫描（nmap 等）可识别"这是 syncthing"：

- **证书指纹**：启动时自签证书，CN 含 `syncthing` 字样；扫端口拉证书即可识别。
- **BEP 握手**：连接后的 hello 消息包含协议名/版本，可被深度检测识别。
- **缓解组合（asuan 已内置/可配）**：
  1. 端口自定义：`stealth.tcp_port` 不用默认 22000（默认已 44312）。
  2. 连接白名单：`stealth.allowed_networks`（经 `asuan firewall add` 写系统防火墙 remoteip）仅允许已知网段 TCP 层接入，扫描者根本进不了握手阶段。
  3. 设备白名单：devices 只含已声明 peers，BEP 握手要求 device ID 匹配。
  4. 发现全关：不广播/不上报，端口仅对主动直连开放。
- 彻底隐藏协议特征需替换引擎内核（不现实），上述组合已覆盖绝大多数主动探测场景。

## 六、开源合规

Syncthing 采用 MPL-2.0。本仓库 [NOTICE](../NOTICE) 记录其来源与许可证；若发行物自带
Syncthing 二进制，须一并携带官方包内 LICENSE.txt / AUTHORS.txt / README.txt。
