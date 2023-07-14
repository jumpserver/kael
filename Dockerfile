FROM golang:1.19-buster as stage-wisp-build
ARG TARGETARCH
ARG GOPROXY=https://goproxy.io
ARG WISP_VERSION=v0.1.11
ENV GO111MODULE=on
ENV CGO_ENABLED=0

RUN --mount=type=cache,target=/root/.cache \
    --mount=type=cache,target=/go/pkg/mod \
    go install github.com/jumpserver/wisp@${WISP_VERSION}

FROM jumpserver/node:18.13 as ui-build
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

FROM jumpserver/python:3.10-slim-buster
ARG TARGETARCH

ARG DEPENDENCIES="                    \
    ca-certificates                   \
    curl                              \
    libssl-dev                        \
    locales                           \
    vim"

ARG APT_MIRROR=http://mirrors.ustc.edu.cn

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked,id=kael \
    sed -i "s@http://.*.debian.org@${APT_MIRROR}@g" /etc/apt/sources.list \
    && rm -f /etc/apt/apt.conf.d/docker-clean \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && apt-get update \
    && apt-get -y install --no-install-recommends ${DEPENDENCIES} \
    && mkdir -p /root/.ssh/ \
    && echo "Host *\n\tStrictHostKeyChecking no\n\tUserKnownHostsFile /dev/null" > /root/.ssh/config \
    && echo "set mouse-=a" > ~/.vimrc \
    && echo "no" | dpkg-reconfigure dash \
    && echo "zh_CN.UTF-8" | dpkg-reconfigure locales \
    && sed -i "s@# export @export @g" ~/.bashrc \
    && sed -i "s@# alias @alias @g" ~/.bashrc \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /tmp/build
COPY ./app/requirements.txt ./requirements/requirements.txt

ARG PIP_MIRROR=https://pypi.douban.com/simple

RUN --mount=type=cache,target=/root/.cache/pip \
    set -ex \
    && pip config set global.index-url ${PIP_MIRROR} \
    && pip install --upgrade pip \
    && pip install --upgrade setuptools wheel \
    && \
    if [ "${TARGETARCH}" == "loong64" ]; then \
        pip install https://download.jumpserver.org/pypi/simple/grpcio/grpcio-1.56.0-cp310-cp310-linux_loongarch64.whl; \
    fi \
    && pip install -r requirements/requirements.txt

WORKDIR /opt/kael

COPY app .
COPY --from=stage-wisp-build /go/bin/wisp /usr/local/bin/wisp

RUN chmod +x ./entrypoint.sh

ARG VERSION
ENV VERSION=$VERSION

ENV LANG=zh_CN.UTF-8

ENV WORK_DIR=/opt/kael
ENV COMPONENT_NAME=kael
ENV EXECUTE_PROGRAM="uvicorn main:app --host 0.0.0.0 --port 8083"

EXPOSE 8083

CMD ["./entrypoint.sh"]
