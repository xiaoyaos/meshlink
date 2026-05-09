# MeshLink production build targets

CLI_NAME := p2p-node
DIST_DIR := dist
BIN_DIR := $(DIST_DIR)/bin
PKG_DIR := $(DIST_DIR)/packages
VERSION ?= 1.0.0
GO_LDFLAGS := -s -w

.PHONY: all clean dist release-cli package-linux package-macos package-windows verify

all: dist

# Full production build of CLI packages for all platforms.
dist: clean release-cli package-linux package-macos package-windows

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

verify:
	go test ./cmd/... ./pkg/...
	go build ./cmd/p2p ./cmd/diag ./pkg/...
	go vet ./cmd/... ./pkg/...

clean:
	rm -rf $(DIST_DIR)
	@echo "Cleaned production build output."
