NAME := kael
BUILD_DIR := build
VERSION ?= dev
LDFLAGS := -s -w -X main.Version=$(VERSION)

.PHONY: build test clean docker

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(NAME) ./cmd/kael

test:
	go test ./...

docker:
	docker buildx build --build-arg VERSION=$(VERSION) -t jumpserver/kael:$(VERSION) .

clean:
	rm -rf $(BUILD_DIR)
