VER ?= $(VERSION)
LDFLAGS = -X "main.BuildTimestamp=$(shell date -u '+%Y-%m-%d %H:%M:%S')" \
          -X "main.VERSION=$(VER)" \
          -X "main.goVersion=$(shell go version | sed -r 's/go version go(.*)\ .*/\1/')"

GO := GO111MODULE=on CGO_ENABLED=0 go
GOLANGCI_LINT_VERSION = v1.61.0

GOOS_LIST := linux windows darwin
GOARCH_LIST := amd64 arm64 386

.PHONY: build
build:
	@mkdir -p bin
	@for goos in $(GOOS_LIST); do \
		if [ "$$goos" = "windows" ]; then \
			ext=".exe"; \
		else \
			ext=""; \
		fi; \
		for goarch in $(GOARCH_LIST); do \
			echo "Building for $$goos-$$goarch..."; \
			GOOS=$$goos GOARCH=$$goarch $(GO) build -ldflags '$(LDFLAGS)' -o bin/BPB-Wizard-$$goos-$$goarch$$ext || echo "Not supported, Skipping $$goos-$$goarch"; \
		done; \
	done
