#!/bin/bash
# push-hub.sh — 构建并推送 asuan-keeper 镜像到任意镜像仓库(registry)。
#
# 用法:
#   ./deploy/hub/push-hub.sh <镜像仓库路径> [版本]
#     镜像仓库路径: 可以是 Docker Hub 用户名/项目,也可以是任意 registry 前缀
#     版本:         镜像标签(默认 latest)
#
# 示例:
#   Docker Hub:  ./deploy/hub/push-hub.sh suantea/asuan-keeper latest
#   阿里云 ACR:  ./deploy/hub/push-hub.sh registry.cn-hangzhou.aliyuncs.com/ns/asuan-keeper v1.0.0
#   腾讯云 TCR:  ./deploy/hub/push-hub.sh ccr.ccs.tencent-cloud.com/ns/asuan-keeper v1.0.0
#   华为云 SWR:  ./deploy/hub/push-hub.sh swr.cn-north-4.myhuaweicloud.com/ns/asuan-keeper v1.0.0
#
# 注意:
# - 登录凭据只在执行时由你本机输入 docker login,脚本不会保存/上传任何账号信息。
# - 需在有 Docker 的机器上执行(本机无 docker 时无法推送)。
# - 国内 registry 首次使用前先创建命名空间与仓库,并在对应控制台开通公网访问。
set -euo pipefail

IMAGE_PATH="${1:-}"
VERSION="${2:-latest}"
if [ -z "$IMAGE_PATH" ]; then
  echo "用法: ./deploy/hub/push-hub.sh <镜像仓库路径> [版本]"
  echo "示例: ./deploy/hub/push-hub.sh registry.cn-hangzhou.aliyuncs.com/ns/asuan-keeper v1.0.0"
  exit 1
fi

DIR="$(cd "$(dirname "$0")" && pwd)"
# Dockerfile 内 COPY 路径相对仓库根（如 deploy/hub/entrypoint.sh），
# 因此 build context 必须是仓库根，而不是 deploy/hub/ 本身。
ROOT="$(cd "$DIR/../.." && pwd)"
IMAGE="$IMAGE_PATH:$VERSION"

echo "==> 构建镜像: $IMAGE (context=$ROOT)"
docker build -f "$DIR/Dockerfile" -t "$IMAGE" "$ROOT"

echo "==> 登录镜像仓库(输入你的用户名与访问令牌/密码)"
# 国内 registry 需先 docker login <registry域名>;Docker Hub 直接 docker login
REGISTRY="${IMAGE_PATH%%/*}"
if [[ "$REGISTRY" == *"."* ]] || [[ "$REGISTRY" == *":"* ]]; then
  docker login "$REGISTRY"
else
  docker login
fi

echo "==> 推送: $IMAGE"
docker push "$IMAGE"

echo "==> 完成。在 docker-compose.yml 中用 image: $IMAGE 替代 build: . 即可拉取运行"
