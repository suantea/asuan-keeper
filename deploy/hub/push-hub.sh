#!/bin/bash
# push-hub.sh — 构建并推送 asuan-hub 镜像到 Docker Hub。
#
# 用法:
#   ./deploy/hub/push-hub.sh <DockerHub用户名> [版本]
#     用户名: 你的 Docker Hub 用户名(必填)
#     版本:    镜像标签(默认 latest)
#
# 示例:
#   ./deploy/hub/push-hub.sh myname latest
#
# 注意:
# - 登录凭据只在执行时由你本机输入 docker login,脚本不会保存/上传任何账号信息。
# - 需在有 Docker 的机器上执行(本机无 docker 时无法推送)。
set -euo pipefail

USERNAME="${1:-}"
VERSION="${2:-latest}"
if [ -z "$USERNAME" ]; then
  echo "用法: ./deploy/hub/push-hub.sh <DockerHub用户名> [版本]"
  exit 1
fi

DIR="$(cd "$(dirname "$0")" && pwd)"
IMAGE="$USERNAME/asuan-hub:$VERSION"

echo "==> 构建镜像: $IMAGE"
docker build -t "$IMAGE" "$DIR"

echo "==> 登录 Docker Hub(输入你的用户名与访问令牌/密码)"
docker login

echo "==> 推送: $IMAGE"
docker push "$IMAGE"

echo "==> 完成。在 docker-compose.yml 中用 image: $IMAGE 替代 build: . 即可拉取运行"
