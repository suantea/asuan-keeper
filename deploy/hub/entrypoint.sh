#!/bin/sh
# asuan-keeper 入口：直接用 asuan 管理 syncthing sidecar。
# 覆盖了官方 syncthing 镜像的 ENTRYPOINT，因此在这里补齐其权限逻辑：
# 以 root 运行时，按 PUID/PGID（compose 环境变量）修正挂载数据目录属主，
# 避免容器与宿主机(NAS)用户权限不一致导致写失败。
set -e

if [ "$(id -u)" = "0" ] && [ -n "$PUID" ] && [ -n "$PGID" ]; then
    chown -R "$PUID:$PGID" /var/syncthing /etc/asuan /sync 2>/dev/null || true
fi

# 首次部署：挂载目录里还没有 asuan.json 时，先 init 生成默认配置，
# 避免 asuan run 因找不到配置直接退出（容器崩溃重启循环）。
if [ ! -f /etc/asuan/asuan.json ]; then
    echo "==> 未找到 /etc/asuan/asuan.json，执行 asuan init 生成默认配置..."
    asuan -config /etc/asuan/asuan.json init || {
        echo "==> init 失败，请检查 /etc/asuan 是否可写（PUID/PGID 是否与宿主机一致）"
        exit 1
    }
    echo "==> 已生成默认配置；如需连接其它设备，请编辑挂载目录下的 asuan.json 后重启容器。"
fi

exec asuan -config /etc/asuan/asuan.json run
