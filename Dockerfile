FROM golang:1.26-bookworm AS builder
ARG TARGETARCH

WORKDIR /opt/kael
RUN wget https://github.com/jumpserver-dev/healthcheck/releases/latest/download/check_linux_${TARGETARCH}.deb \
    && dpkg -i check_linux_${TARGETARCH}.deb \
    && rm -f check_linux_${TARGETARCH}.deb
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /opt/kael/kael ./cmd/kael

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /opt/kael
COPY --from=builder /opt/kael/kael ./kael
COPY --from=builder /usr/local/bin/check /usr/local/bin/check
COPY config_example.yml ./config_example.yml
COPY entrypoint.sh ./entrypoint.sh
RUN chmod 0755 ./entrypoint.sh ./kael
EXPOSE 8083
ENTRYPOINT ["./entrypoint.sh"]
