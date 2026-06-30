BINARY := avm
PKG := github.com/MikD1/agent-vm
SHELL_SCRIPTS := install.sh internal/modules/scripts/*.sh

.PHONY: build test vet shellcheck lint all
all: vet test build

build:
	go build -o bin/$(BINARY) ./cmd/avm

test:
	go test ./...

vet:
	go vet ./...

shellcheck:
	shellcheck $(SHELL_SCRIPTS)

lint: vet shellcheck
