BINARY := avm
PKG := github.com/MikD1/agent-vm
SHELL_SCRIPTS := install.sh internal/provision/scripts/*.sh

GO_DIRS := cmd internal

.PHONY: build test fmt fmt-check vet shellcheck lint all
all: fmt-check vet test build

build:
	go build -o bin/$(BINARY) ./cmd/avm

test:
	go test ./...

fmt:
	gofmt -w $(GO_DIRS)

# Formatting is a CI gate, not a suggestion: fail with the offending files
# listed, so `make fmt` is the obvious next step.
fmt-check:
	@files="$$(gofmt -l $(GO_DIRS))"; \
	if [ -n "$$files" ]; then \
		echo "gofmt needed (run: make fmt):"; \
		echo "$$files"; \
		exit 1; \
	fi

vet:
	go vet ./...

shellcheck:
	shellcheck $(SHELL_SCRIPTS)

lint: fmt-check vet shellcheck
