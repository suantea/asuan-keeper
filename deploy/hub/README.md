# asuan-hub 部署说明（NAS / Docker）

常驻中转节点，持有全量内容 + 回收站（Syncthing trashcan 保留 30 天）。

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
