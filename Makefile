# P2P Virtual Mesh Network Makefile
BINARY_NAME=p2p-gui
CLI_NAME=p2p-node
DIST_DIR=release
VERSION=1.0.0

.PHONY: all clean dist release-cli release-gui release-gui-windows docker-builder package-linux

all: dist

# --- 核心发布任务 ---
# 注意: release-gui 需要在对应平台上运行（macOS 编 .app，Windows 编 .exe）
# 全平台自动化请使用 GitHub Actions: .github/workflows/release.yml
dist: clean release-cli release-gui release-gui-windows

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
	@rm -rf cmd/p2p-desktop/frontend/node_modules
	@mkdir -p cmd/p2p-desktop/frontend/dist
	@touch cmd/p2p-desktop/frontend/dist/.gitkeep
	cd cmd/p2p-desktop && wails build -platform darwin/universal -ldflags="-s -w"
	rm -rf $(DIST_DIR)/gui/$(BINARY_NAME)-macos.app
	mv cmd/p2p-desktop/build/bin/p2p-desktop.app $(DIST_DIR)/gui/$(BINARY_NAME)-macos.app
	@echo "Bundling CLI nodes into macOS App..."
	cp $(DIST_DIR)/cli/$(CLI_NAME)-darwin-arm64 $(DIST_DIR)/gui/$(BINARY_NAME)-macos.app/Contents/MacOS/
	cp $(DIST_DIR)/cli/$(CLI_NAME)-darwin-amd64 $(DIST_DIR)/gui/$(BINARY_NAME)-macos.app/Contents/MacOS/
	@echo "✅ macOS GUI ready: $(DIST_DIR)/gui/$(BINARY_NAME)-macos.app"

# --- Windows GUI：通过 Docker + mingw-w64 在 macOS 上交叉编译 ---
# 首次运行需先构建 Docker 镜像: make docker-builder
docker-builder:
	docker build -f Dockerfile.windows-build -t meshlink-win-builder .
	@echo "✅ Windows builder image ready"

release-gui-windows:
	@mkdir -p $(DIST_DIR)/gui
	docker run --rm \
		-v "$(PWD)":/workspace \
		-w /workspace/cmd/p2p-desktop \
		-e CGO_ENABLED=1 \
		-e CC=x86_64-w64-mingw32-gcc \
		-e CXX=x86_64-w64-mingw32-g++ \
		meshlink-win-builder \
		sh -c "cd frontend && rm -rf node_modules dist && npm ci && npm run build && cd .. && wails build -platform windows/amd64 -ldflags='-s -w' -skipbindings -s"
	cp cmd/p2p-desktop/build/bin/p2p-desktop.exe $(DIST_DIR)/gui/$(BINARY_NAME)-windows-amd64.exe
	@echo "Bundling CLI node and wintun.dll for Windows..."
	cp $(DIST_DIR)/cli/$(CLI_NAME)-windows-amd64.exe $(DIST_DIR)/gui/
	cp pkg/tun/wintun.dll $(DIST_DIR)/gui/
	@echo "✅ Windows GUI ready: $(DIST_DIR)/gui/$(BINARY_NAME)-windows-amd64.exe"

# --- 打包 Linux 发行包（二进制 + 安装脚本）---
package-linux:
	@bash scripts/package-linux.sh $(VERSION)

clean:
	rm -rf $(DIST_DIR)
	@echo "Cleaned."
