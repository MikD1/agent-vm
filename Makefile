BINARY := avm
PKG := github.com/MikD1/agent-vm
SHELL_SCRIPTS := install.sh internal/modules/scripts/*.sh
INSTALL_DIR ?= $(HOME)/.local/bin
MODULES_DST ?= $(HOME)/.config/agent-vm/modules.d

.PHONY: build test vet shellcheck lint all install
all: vet test build

build:
	go build -o bin/$(BINARY) ./cmd/avm

install: build
	mkdir -p $(INSTALL_DIR) $(MODULES_DST)
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@if ls local/modules.d/*.sh 2>/dev/null | grep -q .; then \
		cp local/modules.d/*.sh $(MODULES_DST)/; \
		echo "Custom modules installed to $(MODULES_DST)"; \
	fi
	@echo "$(BINARY) installed to $(INSTALL_DIR)/$(BINARY)"

test:
	go test ./...

vet:
	go vet ./...

shellcheck:
	shellcheck $(SHELL_SCRIPTS)

lint: vet shellcheck
