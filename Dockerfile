FROM jumpserver/kael-base:20260906_150237 AS stage-build
ARG TARGETARCH

WORKDIR /opt/kael
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /opt/kael/kael ./cmd/kael

FROM node:22-trixie-slim AS stage-codex
RUN npm install --global @openai/codex@0.153.2 \
    && codex --version

FROM node:22-trixie-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=stage-codex /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s /usr/local/lib/node_modules/@openai/codex/bin/codex.js /usr/local/bin/codex
WORKDIR /opt/kael
COPY --from=stage-build /opt/kael/kael ./kael
COPY --from=stage-build /usr/local/bin/check /usr/local/bin/check
COPY config_example.yml ./config_example.yml
COPY entrypoint.sh ./entrypoint.sh
RUN chmod 0755 ./entrypoint.sh ./kael
EXPOSE 8083
ENTRYPOINT ["./entrypoint.sh"]
