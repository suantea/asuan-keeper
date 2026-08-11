#!/bin/sh
# asuan-hub 入口：直接用 asuan 管理 syncthing sidecar。
set -e
exec asuan -config /etc/asuan/asuan.json run
