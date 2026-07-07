.PHONY: help build run test clean docker-build-app docker-build-docreader docker-build-frontend docker-build-all docker-run docker-stop docker-restart migrate-up migrate-down migrate-version migrate-create migrate-force migrate-goto build-images build-images-app build-images-docreader build-images-frontend clean-images show-platform restart-infra stop-infra restart-backend restart-all stop-backend dev-logs-app docs install-swagger fmt lint deps build-prod download_spatial clean-db admin-build admin-drop-summary admin-drop-legacy-kb admin-inspect-summary

# Show help
help:
	@echo "WeKnora Makefile 帮助"
	@echo ""
	@echo "基础命令:"
	@echo "  build             构建应用"
	@echo "  run               运行应用"
	@echo "  test              运行测试"
	@echo "  clean             清理构建文件"
	@echo ""
	@echo "Docker 镜像构建:"
	@echo "  docker-build-app       构建应用 Docker 镜像 (wechatopenai/weknora-app)"
	@echo "  docker-build-docreader 构建文档读取器镜像 (wechatopenai/weknora-docreader)"
	@echo "  docker-build-frontend  构建前端镜像 (wechatopenai/weknora-ui)"
	@echo "  docker-build-all       构建所有 Docker 镜像"
	@echo "  build-images           从源码构建所有镜像 (scripts/build_images.sh)"
	@echo "  clean-images           清理本地镜像"
	@echo ""
	@echo "本地开发启停（推荐）:"
	@echo "  restart-infra     启动/重启基础设施容器 (postgres/redis/milvus/docreader)"
	@echo "  stop-infra        停止基础设施容器"
	@echo "  restart-backend   重新编译并重启前后端（保持基础设施不变）"
	@echo "  restart-all       重启基础设施 + 前后端"
	@echo "  stop-backend      仅停止后端/前端进程"
	@echo "  dev-logs-app      查看后端实时日志 (/tmp/weknora-app.log)"
	@echo ""
	@echo "数据库:"
	@echo "  migrate-up        执行数据库迁移"
	@echo "  migrate-down      回滚数据库迁移"
	@echo ""
	@echo "开发工具:"
	@echo "  fmt               格式化代码"
	@echo "  lint              代码检查"
	@echo "  deps              安装依赖"
	@echo "  docs              生成 Swagger API 文档"
	@echo "  install-swagger   安装 swag 工具"
	@echo "  show-platform     显示当前构建平台"

# Go related variables
BINARY_NAME=WeKnora
MAIN_PATH=./cmd/server

# Docker related variables
DOCKER_IMAGE=wechatopenai/weknora-app
DOCKER_TAG=latest

# Platform detection
ifeq ($(shell uname -m),x86_64)
    PLATFORM=linux/amd64
else ifeq ($(shell uname -m),aarch64)
    PLATFORM=linux/arm64
else ifeq ($(shell uname -m),arm64)
    PLATFORM=linux/arm64
else
    PLATFORM=linux/amd64
endif

# Build the application
build:
	go build -o $(BINARY_NAME) $(MAIN_PATH)

# Run the application
run: build
	./$(BINARY_NAME)

# Run tests
test:
	go test -v ./...

# Build the admin maintenance CLI (Milvus cleanup / inspection)
admin-build:
	go build -o weknora-admin ./cmd/admin

# Drop the global weknora_summary collection (used before reingesting summaries)
admin-drop-summary: admin-build
	./weknora-admin drop-summary

# Drop the pre-refactor global weknora_kb collection (post-migration cleanup)
admin-drop-legacy-kb: admin-build
	./weknora-admin drop-legacy-kb

# Inspect the global weknora_summary collection state
admin-inspect-summary: admin-build
	./weknora-admin inspect-summary

# Clean build artifacts
clean:
	go clean
	rm -f $(BINARY_NAME)

# Build Docker image
docker-build-app:
	@echo "获取版本信息..."
	@eval $$(./scripts/get_version.sh env); \
	./scripts/get_version.sh info; \
	docker build --platform $(PLATFORM) \
		--build-arg VERSION_ARG="$$VERSION" \
		--build-arg COMMIT_ID_ARG="$$COMMIT_ID" \
		--build-arg BUILD_TIME_ARG="$$BUILD_TIME" \
		--build-arg GO_VERSION_ARG="$$GO_VERSION" \
		-f docker/Dockerfile.app -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Build docreader Docker image
docker-build-docreader:
	docker build --platform $(PLATFORM) -f docker/Dockerfile.docreader -t wechatopenai/weknora-docreader:latest .

# Build frontend Docker image
docker-build-frontend:
	docker build --platform $(PLATFORM) -f frontend/Dockerfile -t wechatopenai/weknora-ui:latest frontend/

# Build all Docker images
docker-build-all: docker-build-app docker-build-docreader docker-build-frontend

# Run Docker container (compose up)
docker-run:
	docker compose up -d

# Stop Docker container
docker-stop:
	docker compose down

# Restart Docker container
docker-restart:
	docker compose down -t 60
	docker compose up -d

# 从源码构建镜像相关命令
build-images:
	./scripts/build_images.sh

build-images-app:
	./scripts/build_images.sh --app

build-images-docreader:
	./scripts/build_images.sh --docreader

build-images-frontend:
	./scripts/build_images.sh --frontend

clean-images:
	./scripts/build_images.sh --clean

# Database migrations
migrate-up:
	./scripts/migrate.sh up

migrate-down:
	./scripts/migrate.sh down

migrate-version:
	./scripts/migrate.sh version

migrate-create:
	@if [ -z "$(name)" ]; then \
		echo "Error: migration name is required"; \
		echo "Usage: make migrate-create name=your_migration_name"; \
		exit 1; \
	fi
	./scripts/migrate.sh create $(name)

migrate-force:
	@if [ -z "$(version)" ]; then \
		echo "Error: version is required"; \
		echo "Usage: make migrate-force version=4"; \
		exit 1; \
	fi
	./scripts/migrate.sh force $(version)

migrate-goto:
	@if [ -z "$(version)" ]; then \
		echo "Error: version is required"; \
		echo "Usage: make migrate-goto version=3"; \
		exit 1; \
	fi
	./scripts/migrate.sh goto $(version)

# Generate API documentation (Swagger)
docs:
	@echo "生成 Swagger API 文档..."
	swag init -g $(MAIN_PATH)/main.go -o ./docs --parseDependency --parseInternal
	@echo "文档已生成到 ./docs 目录"
	@echo "启动服务后访问 http://localhost:8080/swagger/index.html 查看文档"

# Install swagger tool
install-swagger:
	go install github.com/swaggo/swag/cmd/swag@latest

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Install dependencies
deps:
	go mod download

# Build for production
# google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn for qdrant milvus proto conflict
build-prod:
	VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || echo "$${VERSION:-unknown}"); \
	COMMIT_ID=$${COMMIT_ID:-unknown}; \
	CGO_ENABLED=1 \
	CGO_CFLAGS="-Wno-deprecated-declarations" \
	CGO_LDFLAGS="-Wl,-no_warn_duplicate_libraries" \
	BUILD_TIME=$${BUILD_TIME:-unknown}; \
	GO_VERSION=$${GO_VERSION:-unknown}; \
	LDFLAGS="-X 'github.com/Tencent/WeKnora/internal/handler.Version=$$VERSION' -X 'github.com/Tencent/WeKnora/internal/handler.Edition=standard' -X 'github.com/Tencent/WeKnora/internal/handler.CommitID=$$COMMIT_ID' -X 'github.com/Tencent/WeKnora/internal/handler.BuildTime=$$BUILD_TIME' -X 'github.com/Tencent/WeKnora/internal/handler.GoVersion=$$GO_VERSION' -X 'google.golang.org/protobuf/reflect/protoregistry.conflictPolicy=warn'"; \
	go build -ldflags="-w -s $$LDFLAGS" -o $(BINARY_NAME) $(MAIN_PATH)

download_spatial:
	go run cmd/download/duckdb/duckdb.go

clean-db:
	@echo "Cleaning database..."
	@if [ $$(docker volume ls -q -f name=weknora_postgres-data) ]; then \
		docker volume rm weknora_postgres-data; \
	fi
	@if [ $$(docker volume ls -q -f name=weknora_minio_data) ]; then \
		docker volume rm weknora_minio_data; \
	fi
	@if [ $$(docker volume ls -q -f name=weknora_redis_data) ]; then \
		docker volume rm weknora_redis_data; \
	fi

# Show current platform
show-platform:
	@echo "当前系统架构: $(shell uname -m)"
	@echo "Docker构建平台: $(PLATFORM)"

# ===== 本地开发启停（基础设施 + 前后端）=====

# 启动/重启基础设施容器（postgres/redis/milvus/docreader）
restart-infra:
	./scripts/restart-infra.sh

# 停止基础设施容器
stop-infra:
	./scripts/restart-infra.sh --stop

# 重新编译并重启前后端（保持基础设施不变）
restart-backend:
	./scripts/restart-backend.sh

# 重启前后端 + 同时重启基础设施容器
restart-all:
	./scripts/restart-backend.sh --infra

# 仅停止后端/前端进程
stop-backend:
	./scripts/restart-backend.sh --stop

# 查看后端实时日志
dev-logs-app:
	tail -f /tmp/weknora-app.log
