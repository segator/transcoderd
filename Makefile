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


.DEFAULT: help
.PHONY: help
help:	## show this help menu.
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@@egrep -h "#[#]" $(MAKEFILE_LIST) | sed -e 's/\\$$//' | awk 'BEGIN {FS = "[:=].*?#[#] "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

.PHONY: build
build: buildgo-server buildgo-worker buildcontainer-server buildcontainer-worker  ## Build all artifacts



.PHONY: buildgo-%
buildgo-%:
	@echo "Building dist/transcoderd-$*"
	@CGO_ENABLED=0 go build  -ldflags "-X main.ApplicationName=transcoderd-$* -X main.Version=${PROJECT_VERSION} -X main.Commit=${GIT_COMMIT_SHA} -X main.Date=${BUILD_DATE}" -o dist/transcoderd-$* $*/main.go

.PHONY: publish
publish: publishcontainer-server publishcontainer-worker ## Publish all artifacts


DOCKER_BUILD_ARG := --cache-to type=inline


.PHONY: buildcontainer-%
.PHONY: publishcontainer-%
buildcontainer-% publishcontainer-%:
	@export DOCKER_BUILD_ARG="$(DOCKER_BUILD_ARG) $(if $(findstring publishcontainer,$@),--push,--load)"; \
	docker buildx build \
		$${DOCKER_BUILD_ARG} \
		--cache-from $(IMAGE_NAME):$*-main \
		--cache-from $(IMAGE_NAME):$*-$(GIT_BRANCH_NAME) \
		-t $(IMAGE_NAME):$*-$(PROJECT_VERSION) \
		-f Dockerfile \
		--target $* \
		. ;

