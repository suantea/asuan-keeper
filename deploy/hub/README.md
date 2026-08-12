# asuan-hub 部署说明（NAS / Docker）

常驻中转节点，持有全量内容 + 回收站（Syncthing trashcan 保留 30 天）。

## 端口清单

| 端口 | 用途 | 暴露范围 |
|------|------|----------|
| `stealth.tcp_port`（如 44312） | Syncthing 同步端口，各端必须一致 | 局域网（NAS 防火墙需放行） |
| `web.bind`（默认 0.0.0.0:18084） | asuan 网页控制台 | 局域网 `http://<NAS>:18084` |
| `gui_bind`（默认 127.0.0.1:8384） | Syncthing 原生 GUI | 仅容器内，不暴露 |

## 准备

1. 复制 `asuan.example.json` 为 `asuan.json`，修改：
   - `syncthing.gui_api_key`：随机长字符串（各端不要求一致，hub 只供容器内 asuan 用）
   - `stealth.tcp_port`：监听端口（各端与 hub 保持一致）
   - `peers`：各端设备 ID 与静态地址
   - `folders`：本节点要持有的文件夹，`path` 填容器内路径 `/sync/<id>`
2. 启动：
   ```bash
   docker compose up -d --build
   ```
3. 首次会生成 syncthing 配置并应用隐蔽选项；之后 `asuan run` 每次启动会再次核对配置。

## 发布镜像到 Docker Hub（可选）

默认 `docker compose up --build` 在本地构建。若想推送到 Docker Hub 供多台 NAS 直接拉取：

```bash
# 1. 构建并打标签（把 <user> 换成你的 Docker Hub 用户名）
docker build -t <user>/asuan-hub:<版本> .

# 2. 登录 Docker Hub（凭据只在本机输入，不会写入仓库）
docker login

# 3. 推送
docker push <user>/asuan-hub:<版本>
```

然后改 `docker-compose.yml`，把 `build: .` 换成镜像引用即可在任意 NAS 拉取运行：

```yaml
services:
  hub:
    image: <user>/asuan-hub:<版本>   # 替代 build: .
    container_name: asuan-hub
    ...
```

注意：镜像基于官方 `syncthing/syncthing:2.1.3`（MPL-2.0），镜像内已含 Syncthing 二进制与许可证声明；对外发布请保留 `NOTICE` 中的来源与许可证信息（Dockerfile 未 COPY NOTICE 进镜像，如需随镜像分发可自行添加）。

## 防火墙放行（NAS 侧）

同步端口必须在 NAS 防火墙上对局域网放行，否则对端无法直连 hub。以常见 NAS 为例：

- **群晖 DSM**：控制面板 → 安全性 → 防火墙 → 允许规则，放行 TCP `stealth.tcp_port`（如 44312）。
- **QNAP**：控制面板 → 网络与虚拟交换机 → 防火墙，放行 TCP `stealth.tcp_port`。
- **通用 Linux / 路由器**：
  ```bash
  # 若 NAS 自带防火墙（ufw / firewalld）
  ufw allow 44312/tcp        # 按实际 stealth.tcp_port 替换
  # 若 hub 所在网段与对端不同，还需在路由器上做端口转发：
  # 外部端口 44312 → NAS 内网 IP:44312
  ```
- 验证放行是否生效（在局域网另一台机器上）：
  ```bash
  nc -vz <NAS_IP> 44312
  ```

## 查看状态

```bash
docker exec asuan-hub asuan -config /etc/asuan/asuan.json status
docker logs -f asuan-hub
```

## 网页控制台

`asuan run` 会同时启动内置网页控制台（监听 `web.bind`，hub 默认 `0.0.0.0:18084`）。浏览器打开 `http://<NAS>:18084` 可查看同步状态、编辑配置并保存重载。

## 数据

| 挂载 | 说明 |
|------|------|
| `./syncthing-config` | syncthing 配置/证书 |
| `./data` | syncthing 数据 |
| `./files/<id>` | 平铺明文文件，可直接 SMB/QNAP 文件管理器访问 |

## 注意事项

- 使用 host 网络，端口不固定映射；GUI 仅容器内 loopback，不暴露。
- 同步端口（`stealth.tcp_port`）需要在 NAS 防火墙上放行给局域网。

## Win ↔ NAS 双端联调（最小验证）

以一台 Windows 客户端 + NAS hub 为例：

1. **NAS 侧**：按上文完成 `asuan.json` 与 `docker compose up -d --build`，确认日志无错误：
   ```bash
   docker logs asuan-hub | tail -20
   docker exec asuan-hub asuan -config /etc/asuan/asuan.json status
   ```
2. **Win 侧**：程序目录放 `asuan.exe` + `syncthing.exe`，执行 `asuan init` 生成 `asuan.json`，填写：
   - `name`：本机名（如 `win-pc`）
   - `stealth.tcp_port`：与 hub 一致（如 44312）
   - `peers`：hub 设备 ID + NAS 局域网地址（`tcp://<NAS_IP>:44312`）
   - `folders`：与 hub 相同的 `id`，`path` 为本机路径，`policy: sync`
3. **交换设备 ID**：`asuan status` 打印各自设备 ID，互填进对端的 `peers`，两端重载配置。
4. **Win 侧防火墙**（避免首次运行弹窗）：管理员执行 `asuan firewall add` 预置放行规则（规则名 `asuan-sync-<端口>`，仅按端口放行、不暴露引擎进程名）；确认 `asuan firewall status` 显示已放行后重启 `asuan run`。
5. **验证**：
   - Win 端 `asuan run` 启动后，打开 `http://127.0.0.1:18084`，对端列表应显示 NAS 在线。
   - 在 Win 的同步文件夹新建文件，NAS 的 `./files/<id>` 应出现对应文件（中文文件名/子目录完好）。
   - NAS 网页 `http://<NAS>:18084` 应显示 Win 在线、无待同步文件。
6. **常见排查**：
   - 对端显示离线 → 检查 NAS 防火墙是否放行 `stealth.tcp_port`（见上文）。
   - Win 首次运行弹"允许访问"对话框 → 管理员执行 `asuan firewall add` 预置规则后重启。
   - 设备 ID 不匹配 → `peers` 里填的是**对端**的 ID，别填成自己。
   - 文件夹不同步 → 两端 `folders[].id` 必须完全一致（含大小写）。
   - 权限报错 → 挂载目录属主与 `PUID/PGID` 不符，调整 compose 环境变量。
