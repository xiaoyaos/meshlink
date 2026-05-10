# MeshLink production build targets

CLI_NAME := p2p-node
DIST_DIR := dist
BIN_DIR := $(DIST_DIR)/bin
PKG_DIR := $(DIST_DIR)/packages
VERSION ?= 1.0.0
GO_LDFLAGS := -s -w
HOST_UID := $(shell id -u)
HOST_GID := $(shell id -g)
HOST_GOMODCACHE := $(shell go env GOMODCACHE)
DOCKER_GOCACHE ?= /tmp/meshlink-go-build-cache

.PHONY: all clean dist release-cli package-linux package-macos package-windows desktop desktop-macos desktop-windows verify

all: dist

# Full production build of CLI and desktop packages for all platforms.
dist: clean release-cli package-linux package-macos package-windows desktop

# Build CLI nodes for all supported platforms and architectures.
release-cli:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="$(GO_LDFLAGS) -X 'main.Version=$(VERSION)'" -o $(BIN_DIR)/$(CLI_NAME)-darwin-arm64  ./cmd/p2p
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS) -X 'main.Version=$(VERSION)'" -o $(BIN_DIR)/$(CLI_NAME)-darwin-amd64  ./cmd/p2p
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS) -X 'main.Version=$(VERSION)'" -o $(BIN_DIR)/$(CLI_NAME)-linux-amd64   ./cmd/p2p
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags="$(GO_LDFLAGS) -X 'main.Version=$(VERSION)'" -o $(BIN_DIR)/$(CLI_NAME)-linux-arm64   ./cmd/p2p
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS) -X 'main.Version=$(VERSION)'" -o $(BIN_DIR)/$(CLI_NAME)-windows-amd64.exe ./cmd/p2p
	@echo "✅ CLI binaries ready in $(BIN_DIR)/"

# Build platform-specific installer packages under dist/packages/.
package-linux: release-cli
	@bash scripts/package-linux.sh $(VERSION)

package-macos: release-cli
	@bash scripts/package-macos.sh $(VERSION)

package-windows: release-cli
	@bash scripts/package-windows.sh $(VERSION)

desktop: desktop-macos desktop-windows

desktop-macos: release-cli
	@bash scripts/package-desktop-macos.sh $(VERSION)

desktop-windows: release-cli
	@if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then \
		bash scripts/package-desktop-windows.sh $(VERSION); \
	elif command -v docker >/dev/null 2>&1; then \
		docker image inspect meshlink-win-builder >/dev/null 2>&1 || docker build -f Dockerfile.windows-build -t meshlink-win-builder .; \
		mkdir -p "$(DOCKER_GOCACHE)"; \
		docker run --rm \
			-u "$(HOST_UID):$(HOST_GID)" \
			-e HOME=/tmp \
			-e GOCACHE=/tmp/go-build-cache \
			-e GOMODCACHE=/go/pkg/mod \
			-v "$(CURDIR):/build" \
			-v "$(HOST_GOMODCACHE):/go/pkg/mod" \
			-v "$(DOCKER_GOCACHE):/tmp/go-build-cache" \
			-w /build \
			meshlink-win-builder \
			bash -lc 'export PATH=/usr/local/go/bin:$$PATH; bash scripts/package-desktop-windows.sh $(VERSION)'; \
	else \
		echo "Missing MinGW-w64 toolchain or Docker; cannot build Windows desktop package." >&2; \
		exit 1; \
	fi

verify:
	go test ./cmd/... ./pkg/...
	go build -buildvcs=false ./cmd/p2p ./cmd/diag ./cmd/desktop ./pkg/...
	go vet ./cmd/... ./pkg/...

clean:
	rm -rf $(DIST_DIR)
	@echo "Cleaned production build output."
