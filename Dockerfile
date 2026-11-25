FROM jumpserver/kael-base:20261022_064521 AS stage-build
ARG TARGETARCH

WORKDIR /opt/kael
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /opt/kael/kael ./cmd/kael

FROM scratch
WORKDIR /opt/kael
COPY --from=stage-build /opt/kael/kael ./kael
ENTRYPOINT ["/opt/kael/kael"]
