BINARY := avm
PKG := github.com/MikD1/agent-vm

.PHONY: build test vet shellcheck lint all
all: vet test build

build:
	go build -o bin/$(BINARY) ./cmd/avm

test:
	go test ./...

vet:
	go vet ./...

shellcheck:
	shellcheck internal/modules/scripts/*.sh

lint: vet shellcheck
