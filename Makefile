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
TARGETARCH ?= amd64

UIDIR=ui
YARNINSTALL=yarn install
YARNBUILD=yarn build

LDFLAGS=-w -s

KAELLDFLAGS+=-X 'main.Buildstamp=$(BuildTime)'
KAELLDFLAGS+=-X 'main.Githash=$(COMMIT)'
KAELLDFLAGS+=-X 'main.Goversion=$(GOVERSION)'

KAELBUILD=CGO_ENABLED=0 go build -trimpath -ldflags "$(KAELLDFLAGS) ${LDFLAGS}"

PLATFORM_LIST = \
	darwin-amd64 \
	darwin-arm64 \
	linux-amd64 \
	linux-arm64

all-arch: $(PLATFORM_LIST)

darwin-amd64:kael-ui
	GOARCH=amd64 GOOS=darwin $(KAELBUILD) -o $(BUILDDIR)/$(NAME)-$@ $(KAELSRCFILE)
	mkdir -p $(BUILDDIR)/$(NAME)-$(VERSION)-$@

	cp $(BUILDDIR)/$(NAME)-$@ $(BUILDDIR)/$(NAME)-$(VERSION)-$@/$(NAME)
	cd $(BUILDDIR) && tar -czvf $(NAME)-$(VERSION)-$@.tar.gz $(NAME)-$(VERSION)-$@

darwin-arm64:kael-ui
	GOARCH=arm64 GOOS=darwin $(KAELBUILD) -o $(BUILDDIR)/$(NAME)-$@ $(KAELSRCFILE)
	mkdir -p $(BUILDDIR)/$(NAME)-$(VERSION)-$@

	cp $(BUILDDIR)/$(NAME)-$@ $(BUILDDIR)/$(NAME)-$(VERSION)-$@/$(NAME)
	cd $(BUILDDIR) && tar -czvf $(NAME)-$(VERSION)-$@.tar.gz $(NAME)-$(VERSION)-$@

linux-amd64:kael-ui
	GOARCH=amd64 GOOS=linux $(KAELBUILD) -o $(BUILDDIR)/$(NAME)-$@ $(KAELSRCFILE)
	mkdir -p $(BUILDDIR)/$(NAME)-$(VERSION)-$@

	cp $(BUILDDIR)/$(NAME)-$@ $(BUILDDIR)/$(NAME)-$(VERSION)-$@/$(NAME)
	cd $(BUILDDIR) && tar -czvf $(NAME)-$(VERSION)-$@.tar.gz $(NAME)-$(VERSION)-$@

linux-arm64:kael-ui
	GOARCH=arm64 GOOS=linux $(KAELBUILD) -o $(BUILDDIR)/$(NAME)-$@ $(KAELSRCFILE)
	mkdir -p $(BUILDDIR)/$(NAME)-$(VERSION)-$@

	cp $(BUILDDIR)/$(NAME)-$@ $(BUILDDIR)/$(NAME)-$(VERSION)-$@/$(NAME)
	cd $(BUILDDIR) && tar -czvf $(NAME)-$(VERSION)-$@.tar.gz $(NAME)-$(VERSION)-$@

linux-loong64:kael-ui
	GOARCH=loong64 GOOS=linux $(KAELBUILD) -o $(BUILDDIR)/$(NAME)-$@ $(KAELSRCFILE)
	mkdir -p $(BUILDDIR)/$(NAME)-$(VERSION)-$@

	cp $(BUILDDIR)/$(NAME)-$@ $(BUILDDIR)/$(NAME)-$(VERSION)-$@/$(NAME)
	cd $(BUILDDIR) && tar -czvf $(NAME)-$(VERSION)-$@.tar.gz $(NAME)-$(VERSION)-$@

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
