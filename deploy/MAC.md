# asuan macOS 客户端部署说明

端上守护进程 + 占位符虚拟层（macFUSE）。与 Windows 客户端同一套代码，仅依赖与挂载行为不同。

## 依赖

1. **macFUSE**（占位符虚拟层运行时）：
   ```bash
   brew install --cask macfuse
   # 或官网下载安装：https://macfuse.github.io/
   ```
   安装后需在「系统设置 → 隐私与安全性」中允许 macFUSE 系统扩展加载，并重启一次。
2. **Syncthing 引擎**（sidecar，绿色单文件）：官方 `syncthing-macos-*.zip`，解压后的 `syncthing` 二进制放程序目录（或指定 `syncthing.bin`）。
3. **asuan 本体**：本机编译（见下）。

## 构建（必须在 macOS 本机）

```bash
# cgofuse 依赖 cgo + macFUSE SDK，无法交叉编译，需在 Mac 上构建
cd asuan-keeper
go build -o asuan ./cmd/asuan
```

> Windows 上 `GOOS=darwin go build` 会因 cgofuse 的 cgo 绑定失败，属正常现象。

## 配置（asuan.json）

程序目录放 `asuan` + `syncthing` + `asuan.json`，示例：

```json
{
  "version": 1,
  "name": "mac-book",
  "syncthing": {
    "bin": "syncthing",
    "gui_bind": "127.0.0.1:8384",
    "gui_api_key": "随机长字符串"
  },
  "web": { "bind": "127.0.0.1:18084" },
  "stealth": {
    "disable_upnp": true,
    "disable_local_discovery": true,
    "disable_global_discovery": true,
    "disable_relay": true,
    "disable_nat": true,
    "tcp_port": 44312
  },
  "placeholder": { "mount": "/tmp/asuan-mnt" },
  "peers": [
    { "name": "nas", "device_id": "AAAA-...-AAAAA", "address": "192.168.1.5:44312" }
  ],
  "folders": [
    { "id": "docs", "label": "文档", "path": "/Users/me/Sync/docs", "policy": "sync" }
  ]
}
```

要点：

- `placeholder.mount` 挂载点：**macFUSE 要求挂载点预先存在**（或由 asuan 自动创建空目录）。已存在的非空目录会报错。
- `stealth.tcp_port` 必须与对端（含 NAS hub）一致。
- 首次可 `asuan init` 生成模板再编辑。

## 运行与验证

```bash
./asuan run          # 前台运行：同步引擎 + 网页控制台 + 占位符虚拟层
./asuan status       # 另开终端查看同步状态
./asuan release docs # 释放文件夹（本地实体删除，对端保留）
ls /tmp/asuan-mnt    # 虚拟层显示对端文件索引，访问即水合
./asuan hydrate docs # 水合：从对端拉回
```

网页控制台：`http://127.0.0.1:18084`。

## 与 Windows / NAS 双端联调

同一套流程：

1. 各端 `asuan status` 打印设备 ID，互填进对端 `peers`。
2. 两端 `folders[].id` 完全一致，`stealth.tcp_port` 一致。
3. NAS 防火墙放行同步端口（见 `hub/README.md`）。
4. Mac 端新建文件 → Windows/NAS 应同步收到；`asuan release docs` 后 Mac 本地释放、虚拟层可浏览对端内容、访问触发水合。

## 注意事项

- **沙盒/Gatekeeper**：若 asuan 由 DMG 分发，需签名或用户右键打开放行；macFUSE 扩展首次加载需系统授权。
- **挂载点清理**：虚拟层卸载后挂载点目录由 macFUSE 保留，属正常行为。
- **平台差异（与 Windows 的对照）**：

| 项 | Windows (WinFsp) | macOS (macFUSE) |
|---|---|---|
| 挂载点 | 必须预先不存在（自动创建） | 必须预先存在（可自动创建空目录） |
| 引擎二进制 | `syncthing.exe` | `syncthing` |
| 数据目录 | 程序目录 `syncthing/` | 同左 |
| 构建 | 本机 go build | 本机 go build（需 cgo/macFUSE SDK） |
