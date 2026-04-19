GO ?= go
GOFMT ?= $(GO)fmt
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
GOOPTS ?=
GOOS ?= $(shell $(GO) env GOHOSTOS)
GOARCH ?= $(shell $(GO) env GOHOSTARCH)
GIT_COMMIT_SHA := $(shell git rev-parse --short HEAD)
GIT_BRANCH_NAME := $(shell git rev-parse --abbrev-ref HEAD | sed 's/[^a-zA-Z0-9._-]/-/g')
BUILD_DATE := $(shell date +%Y-%m-%dT%H:%M:%SZ)
IMAGE_NAME ?= ghcr.io/segator/transcoderd
PROJECT_VERSION ?= $(shell cat version.txt)-dev
CACHE ?= true


.DEFAULT: help
.PHONY: help
help: ## show this help menu.
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@@egrep -h "#[#]" $(MAKEFILE_LIST) | sed -e 's/\\$$//' | awk 'BEGIN {FS = "[:=].*?#[#] "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ---------------------------------------------------------------------------
# Code quality
# ---------------------------------------------------------------------------
.PHONY: fmt
fmt: ## Format Go code
	go fmt ./...

.PHONY: lint
lint: ## Run linters
	@golangci-lint run

.PHONY: lint-fix
lint-fix: ## Run linters with auto-fix
	@golangci-lint run --fix

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------
.PHONY: test
test: ## Run unit tests with coverage
	@mkdir -p build
	$(GO) test -v -coverprofile=build/coverage.out -covermode=atomic ./...

.PHONY: test-race
test-race: ## Run unit tests with race detector
	@mkdir -p build
	$(GO) test -v -race -coverprofile=build/coverage.out -covermode=atomic ./...

.PHONY: test-short
test-short: ## Run unit tests in short mode
	@mkdir -p build
	$(GO) test -v -short ./...

.PHONY: test-integration
test-integration: ## Run integration tests (requires Docker)
	$(GO) test -tags=integration -v -timeout 5m ./integration/...

.PHONY: test-e2e
test-e2e: ## Run Docker e2e test (requires built images + test fixture)
	E2E_SERVER_IMAGE=$(IMAGE_NAME):server-$(GIT_BRANCH_NAME) \
	E2E_WORKER_IMAGE=$(IMAGE_NAME):worker-$(GIT_BRANCH_NAME) \
	$(GO) test -tags=integration -v -timeout 30m -run TestDockerE2E ./integration/...

.PHONY: test-all
test-all: test test-integration ## Run all tests (unit + integration)

.PHONY: test-coverage
test-coverage: test ## Run tests and open coverage in browser
	$(GO) tool cover -html=build/coverage.out

# ---------------------------------------------------------------------------
# Go build
# ---------------------------------------------------------------------------
.PHONY: build
build: buildgo-server buildgo-worker buildcontainer-server buildcontainer-worker ## Build everything (Go + Docker)

.PHONY: buildgo
buildgo: buildgo-server buildgo-worker ## Build Go binaries only

.PHONY: buildgo-%
buildgo-%:
	@echo "Building dist/transcoderd-$*"
	@CGO_ENABLED=0 go build \
		-ldflags "-X main.ApplicationName=transcoderd-$* -X transcoder/version.Version=${PROJECT_VERSION} -X transcoder/version.Commit=${GIT_COMMIT_SHA} -X transcoder/version.Date=${BUILD_DATE}" \
		-o dist/transcoderd-$* $*/main.go

# ---------------------------------------------------------------------------
# Docker — pre-built builder images (FFmpeg ~50min, PGS ~5min)
#
# These are published once and reused by all CI builds via FROM in Dockerfile.
# Rebuild only when FFmpeg version or PGS dependencies change:
#   make publish-builder-ffmpeg
#   make publish-builder-pgs
# ---------------------------------------------------------------------------
.PHONY: publish-builder-ffmpeg
publish-builder-ffmpeg: ## Build & push FFmpeg builder image (~50min)
	docker buildx build --push \
		-t $(IMAGE_NAME):builder-ffmpeg \
		-f Dockerfile --target builder-ffmpeg .

.PHONY: publish-builder-pgs
publish-builder-pgs: ## Build & push PGS builder image (~5min)
	docker buildx build --push \
		-t $(IMAGE_NAME):builder-pgs \
		-f Dockerfile --target builder-pgs .

# ---------------------------------------------------------------------------
# Docker — application images (server, worker)
#
# These pull the pre-built FFmpeg/PGS images automatically via FROM args
# in the Dockerfile — no BuildKit cache needed.
# ---------------------------------------------------------------------------
.PHONY: buildcontainer-%
buildcontainer-%:
	docker buildx build --load \
		$(if $(filter false,$(CACHE)),--no-cache,) \
		-t $(IMAGE_NAME):$*-$(PROJECT_VERSION) \
		-t $(IMAGE_NAME):$*-$(GIT_BRANCH_NAME) \
		-f Dockerfile --target $* .

.PHONY: publishcontainer-%
publishcontainer-%:
	docker buildx build --push \
		$(if $(filter false,$(CACHE)),--no-cache,) \
		-t $(IMAGE_NAME):$*-$(PROJECT_VERSION) \
		-t $(IMAGE_NAME):$*-$(GIT_BRANCH_NAME) \
		-f Dockerfile --target $* .

.PHONY: publish
publish: publishcontainer-server publishcontainer-worker ## Publish server & worker images
