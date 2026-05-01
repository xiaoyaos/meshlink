# P2P Virtual Mesh Network Makefile
BINARY_NAME=p2p-gui
CLI_NAME=p2p-node
DIST_DIR=release
VERSION=1.0.0

.PHONY: all clean dist release-cli release-gui

all: dist

# --- 核心发布任务 ---
# 注意: release-gui 需要在对应平台上运行（macOS 编 .app，Windows 编 .exe）
# 全平台自动化请使用 GitHub Actions: .github/workflows/release.yml
dist: clean release-cli release-gui

# --- 编译 CLI 引导/中继节点（无 CGO，支持全平台交叉编译）---
release-cli:
	@mkdir -p $(DIST_DIR)/cli
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o $(DIST_DIR)/cli/$(CLI_NAME)-darwin-arm64  ./cmd/p2p
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST_DIR)/cli/$(CLI_NAME)-darwin-amd64  ./cmd/p2p
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST_DIR)/cli/$(CLI_NAME)-linux-amd64   ./cmd/p2p
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags="-s -w" -o $(DIST_DIR)/cli/$(CLI_NAME)-linux-arm64   ./cmd/p2p
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(DIST_DIR)/cli/$(CLI_NAME)-windows-amd64.exe ./cmd/p2p
	@echo "✅ CLI binaries ready in $(DIST_DIR)/cli/"

# --- 编译 GUI 桌面端（需要在目标平台本机运行）---
release-gui:
	@mkdir -p $(DIST_DIR)/gui
	cd cmd/p2p-desktop && wails build -platform darwin/universal -ldflags="-s -w"
	rm -rf $(DIST_DIR)/gui/$(BINARY_NAME)-macos.app
	mv cmd/p2p-desktop/build/bin/p2p-desktop.app $(DIST_DIR)/gui/$(BINARY_NAME)-macos.app
	@echo "✅ macOS GUI ready: $(DIST_DIR)/gui/$(BINARY_NAME)-macos.app"
	@echo "⚠️  Windows GUI must be built on a Windows machine (or via GitHub Actions CI)"

clean:
	rm -rf $(DIST_DIR)
	@echo "Cleaned."
