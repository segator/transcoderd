GO ?= go
GOFMT ?= $(GO)fmt
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
GOOPTS ?=
GOOS ?= $(shell $(GO) env GOHOSTOS)
GOARCH ?= $(shell $(GO) env GOHOSTARCH)
GIT_COMMIT_SHA := $(shell git rev-parse --short HEAD)
GIT_BRANCH_NAME := $(shell git rev-parse --abbrev-ref HEAD)
BUILD_DATE := $(shell date +%Y-%m-%dT%H:%M:%SZ)
IMAGE_NAME ?= ghcr.io/segator/transcoderd
PROJECT_VERSION ?= $(shell cat version.txt)-dev
CACHE ?=true


.DEFAULT: help
.PHONY: help
help:	## show this help menu.
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@@egrep -h "#[#]" $(MAKEFILE_LIST) | sed -e 's/\\$$//' | awk 'BEGIN {FS = "[:=].*?#[#] "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

.PHONY: fmt
fmt: ## Code Format
	go fmt  ./...

.PHONY: test
test: ## Run unit tests
	$(GO) test -v -race -coverprofile=coverage.out -covermode=atomic ./...

.PHONY: test-short
test-short: ## Run unit tests in short mode
	$(GO) test -v -short -race ./...

.PHONY: test-coverage
test-coverage: test ## Run tests and show coverage
	$(GO) tool cover -html=coverage.out

.PHONY: lint
lint: ## Linters
	@golangci-lint run

.PHONY: lint-fix
lint-fix: ## Lint fix if possible
	@golangci-lint run --fix


.PHONY: build
build: buildgo-server buildgo-worker buildcontainer-server buildcontainer-worker  ## Build all artifacts



.PHONY: buildgo-%
buildgo-%:
	@echo "Building dist/transcoderd-$*"
	@CGO_ENABLED=0 go build  -ldflags "-X main.ApplicationName=transcoderd-$* -X transcoder/version.Version=${PROJECT_VERSION} -X transcoder/version.Commit=${GIT_COMMIT_SHA} -X transcoder/version.Date=${BUILD_DATE}" -o dist/transcoderd-$* $*/main.go

.PHONY: publish
publish: publishcontainer-server publishcontainer-worker ## Publish all artifacts


DOCKER_BUILD_ARG := --cache-to type=inline


.PHONY: buildcontainer-%
.PHONY: publishcontainer-%
buildcontainer-% publishcontainer-%:
	@export DOCKER_BUILD_ARG="$(DOCKER_BUILD_ARG) $(if $(findstring publishcontainer,$@),--push,--load) $(if $(filter false,$(CACHE)),--no-cache,)"; \
	docker buildx build \
		$${DOCKER_BUILD_ARG} \
		--cache-from $(IMAGE_NAME):$*-$(GIT_BRANCH_NAME) \
		--cache-from $(IMAGE_NAME):$*-main \
		-t $(IMAGE_NAME):$*-$(PROJECT_VERSION) \
		-t $(IMAGE_NAME):$*-$(GIT_BRANCH_NAME) \
		-f Dockerfile \
		--target $* \
		. ;

