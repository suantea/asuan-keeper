#!/bin/sh
# asuan-hub 入口：直接用 asuan 管理 syncthing sidecar。
# 覆盖了官方 syncthing 镜像的 ENTRYPOINT，因此在这里补齐其权限逻辑：
# 以 root 运行时，按 PUID/PGID（compose 环境变量）修正挂载数据目录属主，
# 避免容器与宿主机(NAS)用户权限不一致导致写失败。
set -e

if [ "$(id -u)" = "0" ] && [ -n "$PUID" ] && [ -n "$PGID" ]; then
    chown -R "$PUID:$PGID" /var/syncthing /etc/asuan /sync 2>/dev/null || true
fi

exec asuan -config /etc/asuan/asuan.json run
