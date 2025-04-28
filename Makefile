VER ?= $(VERSION)
LDFLAGS += -X "main.BuildTimestamp=$(shell date -u "+%Y-%m-%d %H:%M:%S")"
LDFLAGS += -X "main.VERSION=$(VER)"
LDFLAGS += -X "main.goVersion=$(shell go version | sed -r 's/go version go(.*)\ .*/\1/')"

GO := GO111MODULE=on CGO_ENABLED=0 go
GOLANGCI_LINT_VERSION = v1.61.0

GOOS_LIST := linux windows darwin
GOARCH_LIST_DARWIN := amd64 arm64
GOARCH_LIST_LINUX := amd64 arm64 386
GOARCH_LIST_WINDOWS := amd64 arm64 386

.PHONY: build
build:
	@mkdir -p bin
	@for goos in $(GOOS_LIST); do \
		if [ "$$goos" = "linux" ]; then \
			ARCH_LIST="$(GOARCH_LIST_LINUX)"; \
		elif [ "$$goos" = "windows" ]; then \
			ARCH_LIST="$(GOARCH_LIST_WINDOWS)"; \
		else \
			ARCH_LIST="$(GOARCH_LIST_DARWIN)"; \
		fi; \
		for goarch in $$ARCH_LIST; do \
			echo "Building for $$goos-$$goarch..."; \
			GOOS=$$goos GOARCH=$$goarch $(GO) build -ldflags '$(LDFLAGS)' -o bin/BPB-Wizard-$$goos-$$goarch || echo "Skipping $$goos-$$goarch"; \
		done; \
	done