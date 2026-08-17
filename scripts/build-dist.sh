#!/bin/bash
# build-dist.sh — 构建 asuan 发行包:自动下载 syncthing 引擎 + 构建 asuan + 打包 zip。
#
# 用法:
#   ./scripts/build-dist.sh [目标平台] [架构]
#     目标平台: windows | darwin | linux (默认当前平台)
#     架构:     amd64 | arm64 (默认当前架构)
#
# 环境变量:
#   ST_VERSION       syncthing 版本(默认 v2.1.3,与 internal/syncthing/version.go 一致)
#   ASUAN_ENGINE_BASE 引擎下载镜像(默认 GitHub releases,可设 ghproxy 镜像)
#
# 说明:
# - 中文/非 ASCII 路径下 Go 模块解析异常,脚本自动在临时 ASCII 目录构建后拷回。
# - 打包内容含 asuan + syncthing + 官方包内 LICENSE.txt/AUTHORS.txt/README.txt
#   + NOTICE + README.md,满足 MPL-2.0 分发要求。
set -euo pipefail

# ---------- 参数 ----------
TARGET_OS="${1:-}"
TARGET_ARCH="${2:-}"
ST_VERSION="${ST_VERSION:-v2.1.3}"
BASE="${ASUAN_ENGINE_BASE:-https://github.com/syncthing/syncthing/releases/download}"

if [ -z "$TARGET_OS" ]; then
  case "$(uname -s)" in
    MINGW*|MSYS*|CYGWIN*) TARGET_OS=windows ;;
    Darwin) TARGET_OS=darwin ;;
    Linux) TARGET_OS=linux ;;
    *) echo "无法识别当前平台,请显式指定: windows|darwin|linux"; exit 1 ;;
  esac
fi
if [ -z "$TARGET_ARCH" ]; then
  case "$(uname -m)" in
    x86_64|amd64) TARGET_ARCH=amd64 ;;
    arm64|aarch64) TARGET_ARCH=arm64 ;;
    *) echo "无法识别当前架构,请显式指定: amd64|arm64"; exit 1 ;;
  esac
fi

case "$TARGET_OS" in
  windows) GOOS=windows; EXT=.zip; EXE=.exe ;;
  darwin)  GOOS=darwin;  EXT=.zip; EXE= ;;
  linux)   GOOS=linux;   EXT=.tar.gz; EXE= ;;
  *) echo "不支持的目标平台: $TARGET_OS"; exit 1 ;;
esac
case "$TARGET_ARCH" in
  amd64|arm64) GOARCH="$TARGET_ARCH" ;;
  *) echo "不支持的架构: $TARGET_ARCH"; exit 1 ;;
esac

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
echo "==> 目标: asuan 发行包 (${TARGET_OS}-${TARGET_ARCH}), syncthing ${ST_VERSION}"

# ---------- 1. 临时 ASCII 构建目录 ----------
TMP="$(mktemp -d -t asuan-dist.XXXXXX)"  # /tmp 或 %TEMP%,通常为 ASCII
trap 'rm -rf "$TMP"' EXIT
SRC="$TMP/src"
mkdir -p "$SRC"

echo "==> 复制源码到临时 ASCII 目录: $SRC"
cp -r "$ROOT/cmd" "$ROOT/internal" "$ROOT/go.mod" "$ROOT/go.sum" "$ROOT/third_party" "$ROOT/NOTICE" "$SRC/"

# ---------- 2. 构建 asuan ----------
echo "==> 构建 asuan (GOOS=$GOOS GOARCH=$GOARCH)"
(cd "$SRC" && CGO_ENABLED="${CGO_ENABLED:-0}" GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags="-s -w" -o "$TMP/asuan$EXE" ./cmd/asuan)

# ---------- 3. 下载并解压 syncthing ----------
ST_FILE="syncthing-${TARGET_OS}-${TARGET_ARCH}-${ST_VERSION}${EXT}"
ST_URL="${BASE}/${ST_VERSION}/${ST_FILE}"
echo "==> 下载 syncthing: $ST_URL"
if ! curl -fsSL -o "$TMP/st${EXT}" "$ST_URL"; then
  echo "下载失败,可设置 ASUAN_ENGINE_BASE 指向 ghproxy 镜像,例如:"
  echo "  ASUAN_ENGINE_BASE=https://ghproxy.net/https://github.com/syncthing/syncthing/releases/download"
  exit 1
fi
echo "==> 解压 syncthing"
mkdir -p "$TMP/st"
if [ "$EXT" = ".tar.gz" ]; then
  tar -xzf "$TMP/st.tar.gz" -C "$TMP/st"
else
  (cd "$TMP/st" && unzip -q "$TMP/st.zip" || tar -xf "$TMP/st.zip")
fi
ST_DIR="$(find "$TMP/st" -maxdepth 2 -type d -name "syncthing-*" | head -1)"
ST_BIN="$(find "$ST_DIR" -maxdepth 1 -type f -name "syncthing$EXE" | head -1)"
if [ -z "$ST_BIN" ]; then
  echo "未在解压包中找到 syncthing 二进制"; exit 1
fi

# ---------- 4. 组装发行目录 ----------
DIST="$ROOT/dist/asuan-${ST_VERSION}-${TARGET_OS}-${TARGET_ARCH}"
rm -rf "$DIST"
mkdir -p "$DIST"
echo "==> 组装发行目录: $DIST"
cp "$TMP/asuan$EXE" "$DIST/asuan$EXE"
cp "$ST_BIN" "$DIST/syncthing$EXE"
# MPL-2.0 分发要求:携带官方包内许可证与来源文件 + 项目 NOTICE/README
for f in LICENSE.txt AUTHORS.txt README.txt; do
  if [ -f "$ST_DIR/$f" ]; then cp "$ST_DIR/$f" "$DIST/"; fi
done
cp "$ROOT/NOTICE" "$DIST/"
cp "$ROOT/README.md" "$DIST/"
if [ -f "$ROOT/deploy/hub/asuan.example.json" ]; then
  cp "$ROOT/deploy/hub/asuan.example.json" "$DIST/asuan.example.json"
fi
# Windows 分发附带双击启动器（自动 init + run + 打开控制台）
if [ "$TARGET_OS" = "windows" ] && [ -f "$ROOT/deploy/启动-asuan.bat" ]; then
  cp "$ROOT/deploy/启动-asuan.bat" "$DIST/启动-asuan.bat"
fi
# Windows 分发附带无窗口后台启动器（双击即后台运行，无需常开窗口）
if [ "$TARGET_OS" = "windows" ] && [ -f "$ROOT/deploy/后台启动-asuan.vbs" ]; then
  cp "$ROOT/deploy/后台启动-asuan.vbs" "$DIST/后台启动-asuan.vbs"
fi
# 快速开始提示
cat > "$DIST/README-QUICKSTART.txt" <<'QS'
asuan 快速开始
================
1. 同目录已有 asuan 与 syncthing 引擎,无需额外下载。
2. 首次运行: 双击「启动-asuan.bat」即可(自动 init + run + 打开网页控制台)
   「后台启动-asuan.vbs」= 无窗口后台运行(双击即常驻,无需保持窗口)
   或命令行: asuan init  (生成 asuan.json 并输出本机设备 ID)
3. 网页控制台 http://127.0.0.1:18084 提供分字段配置表单
   (对端 peers / 文件夹 folders / 网络隐蔽 均可可视化编辑,保存自动生效);
   高级项可切换「JSON 编辑」直接修改 asuan.json。
4. 无需管理员权限: asuan 使用高位端口且数据写在程序目录,普通用户即可运行。
5. 防火墙弹窗(可选规避): 首次运行 syncthing 弹「Windows Defender 已阻止」时,
   勾选「专用网络」→「允许访问」即可一次放行;
   或以管理员运行一次 `asuan firewall add` 预置规则(仅放行同步端口,不暴露程序路径)。
6. 开机自启(可选,免管理员): `asuan autostart on` (登录后隐藏窗口自动启动)
7. 占位符按需拉取(可选): 编辑配置 placeholder.mount 指定挂载点,
   Windows 需先安装 WinFsp(https://winfsp.dev/),未安装时会提示并跳过。
8. 停止: asuan stop  (托盘右键菜单也可退出)
设备交换: 每端 asuan status 显示的设备 ID 填入其他端的 peers。
QS

# ---------- 5. 打包 zip ----------
echo "==> 打包 zip"
(cd "$ROOT/dist" && zip -qr "asuan-${ST_VERSION}-${TARGET_OS}-${TARGET_ARCH}.zip" "$(basename "$DIST")" \
  || tar -czf "asuan-${ST_VERSION}-${TARGET_OS}-${TARGET_ARCH}.tar.gz" "$(basename "$DIST")")

echo "==> 完成"
ls -lh "$ROOT/dist/"
