# asuan-keeper：NAS 常驻中转节点镜像。
# 基于官方 syncthing 镜像，叠加我们的 asuan 管理程序，统一配置路径。
# 构建阶段用 Debian（golang:*-bookworm）而非 alpine：asuan 依赖 cgofuse(FUSE)
# 与 systray(GTK/ayatana)，需要 cgo 及系统头文件，alpine 下 CGO_ENABLED=0 编译失败。
FROM golang:1.26-bookworm AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc libc6-dev libfuse-dev libgtk-3-dev libayatana-appindicator3-dev pkg-config \
    && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
COPY internal ./internal
COPY cmd ./cmd
# go.mod 里 replace github.com/getlantern/systray => ./third_party/systray，
# 构建阶段必须把 third_party 一起复制进去
COPY third_party ./third_party
RUN CGO_ENABLED=1 go build -tags cgofuse -ldflags="-s -w" -o /out/asuan ./cmd/asuan

FROM debian:bookworm-slim
# 运行阶段用 glibc 基础（asuan 是 CGO 动态链接 glibc 二进制，官方 syncthing
# 镜像为 musl/Alpine 系，缺 ld-linux loader 会导致 "asuan: not found"）；
# syncthing 官方二进制是静态链接，可直接 COPY 引入。
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgtk-3-0 libayatana-appindicator3-1 libglib2.0-0 libfuse2 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/asuan /usr/bin/asuan
COPY --from=syncthing/syncthing:2.1.3 /bin/syncthing /usr/bin/syncthing
# context 为仓库根时，entrypoint.sh 在 deploy/hub/ 下
COPY deploy/hub/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
