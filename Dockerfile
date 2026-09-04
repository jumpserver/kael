FROM jumpserver/kael-base:20260904_062559 AS stage-build
ARG TARGETARCH

WORKDIR /opt/kael
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /opt/kael/kael ./cmd/kael

FROM debian:trixie-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /opt/kael
COPY --from=stage-build /opt/kael/kael ./kael
COPY --from=stage-build /usr/local/bin/check /usr/local/bin/check
COPY config_example.yml ./config_example.yml
COPY entrypoint.sh ./entrypoint.sh
RUN chmod 0755 ./entrypoint.sh ./kael
EXPOSE 8083
ENTRYPOINT ["./entrypoint.sh"]
