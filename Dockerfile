FROM golang:1.26-bookworm AS builder

WORKDIR /opt/kael
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
COPY config_example.yml ./config_example.yml
COPY entrypoint.sh ./entrypoint.sh
RUN chmod 0755 ./entrypoint.sh ./kael
EXPOSE 8083
ENTRYPOINT ["./entrypoint.sh"]
