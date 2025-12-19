FROM debian:bullseye-slim as stage-wisp-build
ARG TARGETARCH

ARG DEPENDENCIES="                    \
        ca-certificates               \
        wget"

ARG APT_MIRROR=http://mirrors.ustc.edu.cn
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked,id=chen \
    --mount=type=cache,target=/var/lib/apt,sharing=locked,id=chen \
    set -ex \
    && rm -f /etc/apt/apt.conf.d/docker-clean \
    && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' >/etc/apt/apt.conf.d/keep-cache \
    && sed -i "s@http://.*.debian.org@${APT_MIRROR}@g" /etc/apt/sources.list \
    && apt-get update \
    && apt-get -y install --no-install-recommends ${DEPENDENCIES} \
    && echo "no" | dpkg-reconfigure dash \
    && apt-get clean all \
    && rm -rf /var/lib/apt/lists/*

ARG WISP_VERSION=v0.2.9
RUN set -ex \
    && wget -O /tmp/wisp.tar.gz https://github.com/jumpserver/wisp/releases/download/${WISP_VERSION}/wisp-${WISP_VERSION}-linux-${TARGETARCH}.tar.gz \
    && tar -xf /tmp/wisp.tar.gz -C /usr/local/bin/ --strip-components=1 \
    && chmod 755 /usr/local/bin/wisp \
    && rm -f /tmp/wisp.tar.gz

ARG BUILD_HASH=dev-build
# Override at your own risk - non-root configurations are untested
ARG UID=0
ARG GID=0

######## WebUI frontend ########
FROM --platform=$BUILDPLATFORM node:22-alpine3.20 AS build
ARG BUILD_HASH

# Set Node.js options (heap limit Allocation failed - JavaScript heap out of memory)
ENV NODE_OPTIONS="--max-old-space-size=4096"

WORKDIR /app

# to store git revision in build
RUN apk add --no-cache git

COPY package.json package-lock.json ./
RUN npm ci --force

COPY . .
ENV APP_BUILD_HASH=${BUILD_HASH}
RUN npm run build

######## WebUI backend ########
FROM python:3.11-slim-bookworm AS base

# Use args
ARG UID
ARG GID
ARG PIP_MIRROR=https://pypi.org/simple

## Basis ##
ENV ENV=prod \
    PORT=8083 \
    LANG=en_US.UTF-8 \
    PATH=/opt/py3/bin:$PATH

## Basis URL Config ##
ENV OLLAMA_BASE_URL="/ollama" \
    OPENAI_API_BASE_URL=""

## API Key and Security Config ##
ENV OPENAI_API_KEY="" \
    WEBUI_SECRET_KEY="" \
    SCARF_NO_ANALYTICS=true \
    DO_NOT_TRACK=true \
    ANONYMIZED_TELEMETRY=false

#### Other models ##########################################################

WORKDIR /app

ENV HOME=/root
# Create user and group if not root
RUN if [ $UID -ne 0 ]; then \
    if [ $GID -ne 0 ]; then \
    addgroup --gid $GID app; \
    fi; \
    adduser --uid $UID --gid $GID --home $HOME --disabled-password --no-create-home app; \
    fi

# Make sure the user has access to the app and root directory
RUN chown -R $UID:$GID /app $HOME

# Install common system dependencies
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    git build-essential pandoc gcc netcat-openbsd curl jq \
    python3-dev \
    ffmpeg libsm6 libxext6 \
    && rm -rf /var/lib/apt/lists/*

# install python dependencies (from pyproject.toml)
COPY --chown=$UID:$GID ./pyproject.toml ./uv.lock ./

RUN --mount=type=cache,target=/root/.cache \
    set -ex && \
    pip3 install --no-cache-dir -i ${PIP_MIRROR} uv && \
    uv venv /opt/py3 && \
    uv pip install -i ${PIP_MIRROR} --python /opt/py3/bin/python -r pyproject.toml --no-cache-dir && \
    mkdir -p /app/backend/data && chown -R $UID:$GID /app/backend/data/ && \
    rm -rf /var/lib/apt/lists/*;

# copy embedding weight from build
# RUN mkdir -p /root/.cache/chroma/onnx_models/all-MiniLM-L6-v2
# COPY --from=build /app/onnx /root/.cache/chroma/onnx_models/all-MiniLM-L6-v2/onnx

COPY --from=stage-wisp-build /usr/local/bin/wisp /usr/local/bin/wisp

# copy built frontend files
COPY --chown=$UID:$GID --from=build /app/build /app/build
COPY --chown=$UID:$GID --from=build /app/CHANGELOG.md /app/CHANGELOG.md
COPY --chown=$UID:$GID --from=build /app/package.json /app/package.json

# copy backend files
COPY --chown=$UID:$GID ./backend /app/backend

WORKDIR /app/backend

EXPOSE 8083

# Minimal, atomic permission hardening for OpenShift (arbitrary UID):
# - Group 0 owns /app and /root
# - Directories are group-writable and have SGID so new files inherit GID 0
RUN if [ "$USE_PERMISSION_HARDENING" = "true" ]; then \
    set -eux; \
    chgrp -R 0 /app /root || true; \
    chmod -R g+rwX /app /root || true; \
    find /app -type d -exec chmod g+s {} + || true; \
    find /root -type d -exec chmod g+s {} + || true; \
    fi

USER $UID:$GID

ARG BUILD_HASH
ENV WEBUI_BUILD_VERSION=${BUILD_HASH}
ENV DOCKER=true

CMD [ "wisp"]
CMD [ "bash", "start.sh"]
