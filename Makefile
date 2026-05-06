# MeshLink production build targets

BINARY_NAME := p2p-gui
CLI_NAME := p2p-node
DIST_DIR := dist
BIN_DIR := $(DIST_DIR)/bin
APP_DIR := $(DIST_DIR)/apps
PKG_DIR := $(DIST_DIR)/packages
VERSION ?= 1.0.0
GO_LDFLAGS := -s -w

.PHONY: all clean dist release-cli release-gui release-gui-windows docker-builder package-linux verify

all: dist

# Full production build. GUI targets still require platform-specific toolchains.
dist: clean release-cli release-gui release-gui-windows package-linux

# Build CLI nodes. These binaries are used by servers and bundled by desktop apps.
release-cli:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="$(GO_LDFLAGS)" -o $(BIN_DIR)/$(CLI_NAME)-darwin-arm64  ./cmd/p2p
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS)" -o $(BIN_DIR)/$(CLI_NAME)-darwin-amd64  ./cmd/p2p
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS)" -o $(BIN_DIR)/$(CLI_NAME)-linux-amd64   ./cmd/p2p
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags="$(GO_LDFLAGS)" -o $(BIN_DIR)/$(CLI_NAME)-linux-arm64   ./cmd/p2p
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(GO_LDFLAGS)" -o $(BIN_DIR)/$(CLI_NAME)-windows-amd64.exe ./cmd/p2p
	@echo "CLI binaries ready in $(BIN_DIR)/"

# Build macOS GUI. Run this on macOS with Wails installed.
release-gui: release-cli
	@mkdir -p $(APP_DIR)
	@rm -rf cmd/p2p-desktop/frontend/node_modules
	@mkdir -p cmd/p2p-desktop/frontend/dist
	@touch cmd/p2p-desktop/frontend/dist/.gitkeep
	cd cmd/p2p-desktop && wails build -platform darwin/universal -ldflags="$(GO_LDFLAGS)"
	rm -rf $(APP_DIR)/$(BINARY_NAME)-macos.app
	mv cmd/p2p-desktop/build/bin/p2p-desktop.app $(APP_DIR)/$(BINARY_NAME)-macos.app
	cp $(BIN_DIR)/$(CLI_NAME)-darwin-arm64 $(APP_DIR)/$(BINARY_NAME)-macos.app/Contents/MacOS/
	cp $(BIN_DIR)/$(CLI_NAME)-darwin-amd64 $(APP_DIR)/$(BINARY_NAME)-macos.app/Contents/MacOS/
	@echo "macOS GUI ready: $(APP_DIR)/$(BINARY_NAME)-macos.app"

# Build Docker image used to cross-compile the Windows GUI on macOS/Linux.
docker-builder:
	docker build -f Dockerfile.windows-build -t meshlink-win-builder .
	@echo "Windows builder image ready"

# Build Windows portable GUI bundle. Run docker-builder once before this target.
release-gui-windows: release-cli
	@mkdir -p $(APP_DIR)/windows-amd64
	docker run --rm \
		-v "$(PWD)":/workspace \
		-w /workspace/cmd/p2p-desktop \
		-e CGO_ENABLED=1 \
		-e CC=x86_64-w64-mingw32-gcc \
		-e CXX=x86_64-w64-mingw32-g++ \
		meshlink-win-builder \
		sh -c "cd frontend && rm -rf node_modules dist && npm ci && npm run build && cd .. && wails build -platform windows/amd64 -ldflags='$(GO_LDFLAGS)' -skipbindings -s"
	cp cmd/p2p-desktop/build/bin/p2p-desktop.exe $(APP_DIR)/windows-amd64/$(BINARY_NAME)-windows-amd64.exe
	cp $(BIN_DIR)/$(CLI_NAME)-windows-amd64.exe $(APP_DIR)/windows-amd64/
	cp pkg/tun/wintun.dll $(APP_DIR)/windows-amd64/
	@echo "Windows GUI bundle ready: $(APP_DIR)/windows-amd64/"

# Build Linux installer archive under dist/packages/.
package-linux: release-cli
	@bash scripts/package-linux.sh $(VERSION)

verify:
	go test ./cmd/... ./pkg/...
	go build ./cmd/p2p ./cmd/diag ./pkg/...
	go vet ./cmd/... ./pkg/...

clean:
	rm -rf $(DIST_DIR)
	rm -rf cmd/p2p-desktop/build/bin
	@echo "Cleaned production build output."
