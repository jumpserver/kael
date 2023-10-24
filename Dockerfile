FROM node:16.20-bullseye-slim as ui-build
ARG TARGETARCH
ARG NPM_REGISTRY="https://registry.npmmirror.com"
ENV NPM_REGISTY=$NPM_REGISTRY

RUN set -ex \
    && npm config set registry ${NPM_REGISTRY} \
    && yarn config set registry ${NPM_REGISTRY}

WORKDIR /opt/kael/ui
ADD ui/package.json ui/yarn.lock .
RUN --mount=type=cache,target=/usr/local/share/.cache/yarn,sharing=locked,id=kael \
    yarn install

ADD ui .
RUN --mount=type=cache,target=/usr/local/share/.cache/yarn,sharing=locked,id=kael \
    yarn build

FROM golang:1.21-buster as kael-build
ARG TARGETARCH

WORKDIR /opt/kael

ADD go.mod go.sum .

ARG GOPROXY=https://goproxy.io
ENV CGO_ENABLED=0
ENV GO111MODULE=on
ENV GOOS=linux

RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download -x

COPY . .

ARG VERSION
ENV VERSION=$VERSION

RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    set +x \
    && export GOFlAGS="-X 'main.Buildstamp=`date -u '+%Y-%m-%d %I:%M:%S%p'`'" \
    && export GOFlAGS="$GOFlAGS -X 'main.Githash=`git rev-parse HEAD`'" \
    && export GOFlAGS="${GOFlAGS} -X 'main.Goversion=`go version`'" \
    && export GOFlAGS="$GOFlAGS -X 'main.Version=$VERSION'" \
    && go build -ldflags "$GOFlAGS" -o kael ./cmd/kael \
    && set -x && ls -al .

RUN mkdir /opt/kael/release \
    && chmod +x /opt/kael/entrypoint.sh \
    && mv /opt/kael/entrypoint.sh /opt/kael/release

FROM debian:bullseye-slim
ARG TARGETARCH
ENV LANG=zh_CN.UTF-8

ARG DEPENDENCIES="                    \
        curl                          \
        git                           \
        net-tools                     \
        unzip                         \
        vim                           \
        locales                       \
        wget"

ARG APT_MIRROR=http://mirrors.ustc.edu.cn
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked,id=kael \
    sed -i "s@http://.*.debian.org@${APT_MIRROR}@g" /etc/apt/sources.list \
    && rm -f /etc/apt/apt.conf.d/docker-clean \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && apt-get update \
    && apt-get install -y --no-install-recommends ${DEPENDENCIES} \
    && apt-get update \
    && echo "no" | dpkg-reconfigure dash \
    && echo "zh_CN.UTF-8" | dpkg-reconfigure locales \
    && sed -i "s@# export @export @g" ~/.bashrc \
    && sed -i "s@# alias @alias @g" ~/.bashrc \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /opt

ARG WISP_VERSION=v0.1.16
RUN set -ex \
    && wget https://github.com/jumpserver/wisp/releases/download/${WISP_VERSION}/wisp-${WISP_VERSION}-linux-${TARGETARCH}.tar.gz \
    && tar -xf wisp-${WISP_VERSION}-linux-${TARGETARCH}.tar.gz -C /usr/local/bin/ --strip-components=1 \
    && chown root:root /usr/local/bin/wisp \
    && chmod 755 /usr/local/bin/wisp \
    && rm -f /opt/*.tar.gz

WORKDIR /opt/kael

COPY --from=ui-build /opt/kael/ui/dist ./ui/dist
COPY --from=kael-build /opt/kael/kael .
COPY --from=kael-build /opt/kael/release .

ARG VERSION
ENV VERSION=$VERSION

EXPOSE 8083

CMD ["./entrypoint.sh"]
