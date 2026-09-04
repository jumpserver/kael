NAME := kael
BUILD_DIR := build
VERSION ?= dev
RUN_ARGS ?=
LDFLAGS := -s -w -X main.Version=$(VERSION)

.PHONY: build run test clean docker

build:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(NAME) ./cmd/kael

run:
	go run ./cmd/kael $(RUN_ARGS)

test:
	go test ./...

docker:
	docker buildx build --build-arg VERSION=$(VERSION) -t jumpserver/kael:$(VERSION) .

clean:
	rm -rf $(BUILD_DIR)
