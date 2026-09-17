VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X ai-subscription-keeper/internal/version.Version=$(VERSION)

.PHONY: dev dev-web build web-build docker run test clean version

version:
	@echo $(VERSION)

# 本地开发：仅启动后端（前端 dev server 代理 /api 到 8080）
dev:
	go run ./cmd/server

dev-web:
	cd web && npm run dev

# 构建单二进制（前端编译并嵌入，版本号注入）
build: web-build
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/keeper ./cmd/server
	@echo "bin/keeper ($(VERSION))"

web-build:
	cd web && npm ci && npm run build
	rm -rf internal/webui/dist/*
	cp -R web/dist/* internal/webui/dist/

docker:
	docker build -t ai-subscription-keeper .

test:
	go test ./...

clean:
	rm -rf bin internal/webui/dist
	mkdir -p internal/webui/dist && touch internal/webui/dist/.gitkeep
