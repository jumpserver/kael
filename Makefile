NAME=kael
BUILDDIR=build

BASEPATH := $(shell pwd)
BRANCH := $(shell git symbolic-ref HEAD 2>/dev/null | cut -d"/" -f 3)
BUILD := $(shell git rev-parse --short HEAD)
KAELSRCFILE := $(BASEPATH)/cmd/kael/

VERSION ?= $(BRANCH)-$(BUILD)
BuildTime:= $(shell date -u '+%Y-%m-%d %I:%M:%S%p')
COMMIT:= $(shell git rev-parse HEAD)
GOVERSION:= $(shell go version)

GOOS:=$(shell go env GOOS)
GOARCH:=$(shell go env GOARCH)

UIDIR=ui
YARNINSTALL=yarn install
YARNBUILD=yarn build

LDFLAGS=-w -s

KAELLDFLAGS+=-X 'main.Buildstamp=$(BuildTime)'
KAELLDFLAGS+=-X 'main.Githash=$(COMMIT)'
KAELLDFLAGS+=-X 'main.Goversion=$(GOVERSION)'

KAELBUILD=CGO_ENABLED=0 go build -trimpath -ldflags "$(KAELLDFLAGS) ${LDFLAGS}"

define make_artifact_full
	GOOS=$(1) GOARCH=$(2) $(KAELBUILD) -o $(BUILDDIR)/$(NAME)-$(1)-$(2) $(KAELSRCFILE)
	mkdir -p $(BUILDDIR)/$(NAME)-$(VERSION)-$(1)-$(2)

	cp $(BUILDDIR)/$(NAME)-$(1)-$(2) $(BUILDDIR)/$(NAME)-$(VERSION)-$(1)-$(2)/$(NAME)
	cd $(BUILDDIR) && tar -czvf $(NAME)-$(VERSION)-$(1)-$(2).tar.gz $(NAME)-$(VERSION)-$(1)-$(2)
	rm -rf $(BUILDDIR)/$(NAME)-$(VERSION)-$(1)-$(2) $(BUILDDIR)/$(NAME)-$(1)-$(2)
endef

build:
	@echo "build kael"
	GOARCH=$(GOARCH) GOOS=$(GOOS) $(KAELBUILD) -o $(BUILDDIR)/$(NAME) $(KAELSRCFILE)

all: kael-ui
	$(call make_artifact_full,darwin,amd64)
	$(call make_artifact_full,darwin,arm64)
	$(call make_artifact_full,linux,amd64)
	$(call make_artifact_full,linux,arm64)
	$(call make_artifact_full,linux,ppc64le)
	$(call make_artifact_full,linux,s390x)
	$(call make_artifact_full,linux,riscv64)

local: kael-ui
	$(call make_artifact_full,$(shell go env GOOS),$(shell go env GOARCH))

darwin-amd64: kael-ui
	$(call make_artifact_full,darwin,amd64)

darwin-arm64: kael-ui
	$(call make_artifact_full,darwin,arm64)

linux-amd64: kael-ui
	$(call make_artifact_full,linux,amd64)

linux-arm64: kael-ui
	$(call make_artifact_full,linux,arm64)

linux-loong64: kael-ui
	$(call make_artifact_full,linux,loong64)

linux-ppc64le: kael-ui
	$(call make_artifact_full,linux,ppc64le)

linux-s390x: kael-ui
	$(call make_artifact_full,linux,s390x)

linux-riscv64: kael-ui
	$(call make_artifact_full,linux,riscv64)

kael-ui:
	@echo "build ui"
	@cd $(UIDIR) && $(YARNINSTALL) && $(YARNBUILD)

.PHONY: docker
docker:
	@echo "build docker images"
	docker buildx build --build-arg VERSION=$(VERSION) -t jumpserver/kael .

.PHONY: clean
clean:
	-rm -rf $(BUILDDIR)
	-rm -rf $(UIDIR)/dist/*
