# asuan-hub：NAS 常驻中转节点镜像。
# 基于官方 syncthing 镜像，叠加我们的 asuan 管理程序，统一配置路径。
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY go.mod go.sum ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/asuan ./cmd/asuan

FROM syncthing/syncthing:2.1.3
COPY --from=build /out/asuan /usr/bin/asuan
# context 为仓库根时，entrypoint.sh 在 deploy/hub/ 下
COPY deploy/hub/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
