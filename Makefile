APP := codexswitch
BIN_NAME := ccodex
GOCACHE_DIR ?= /tmp/codexswitch-go-build

.PHONY: build install test clean dist dist-all

build:
	mkdir -p .build
	env GOCACHE=$(GOCACHE_DIR) go build -o .build/$(BIN_NAME) ./cmd/$(APP)

dist:
	./scripts/package.sh

dist-all:
	./scripts/package-all.sh

install:
	./scripts/install.sh

test:
	env GOCACHE=$(GOCACHE_DIR) go test ./...

clean:
	rm -rf .build dist
