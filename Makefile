.PHONY: build clean install test release run

BINARY_NAME=qiniu-logs
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X github.com/rela/qiniu-logs/cmd.version=$(VERSION)"

# Go 参数
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod

# 平台
PLATFORMS=darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) .

# 本地直接运行，免去先编译；用法: make run ARGS="list 12345 --last 24h"
run:
	$(GOCMD) run . $(ARGS)

install: build
	mv $(BINARY_NAME) $(GOPATH)/bin/

clean:
	rm -f $(BINARY_NAME)
	rm -rf dist/

test:
	$(GOTEST) -v ./...

deps:
	$(GOMOD) download
	$(GOMOD) tidy

# 交叉编译所有平台
release: clean
	mkdir -p dist
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		$(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-$${platform%/*}-$${platform#*/}$(if $(findstring windows,$${platform}),.exe) .; \
	done
	@echo "Release binaries created in dist/"

# macOS 专用构建 (用于 Homebrew)
release-darwin:
	mkdir -p dist
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 .
	cd dist && tar -czf $(BINARY_NAME)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64
	cd dist && tar -czf $(BINARY_NAME)-darwin-arm64.tar.gz $(BINARY_NAME)-darwin-arm64
	cd dist && shasum -a 256 *.tar.gz > checksums.txt
	@echo "Darwin release created in dist/"

# 本地安装到 /usr/local/bin
install-local: build
	sudo cp $(BINARY_NAME) /usr/local/bin/
	@echo "Installed to /usr/local/bin/$(BINARY_NAME)"
