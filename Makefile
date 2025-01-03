GO ?= go
GOFMT ?= $(GO)fmt
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
GOOPTS ?=
GOOS ?= $(shell $(GO) env GOHOSTOS)
GOARCH ?= $(shell $(GO) env GOHOSTARCH)

IMAGE_NAME ?= ghcr.io/segator/transcoderd
IMAGE_VERSION ?= latest

PROJECT_VERSION := 0.1.11

.DEFAULT: help
.PHONY: help
help:	## show this help menu.
	@echo "Usage: make [TARGET ...]"
	@echo ""
	@@egrep -h "#[#]" $(MAKEFILE_LIST) | sed -e 's/\\$$//' | awk 'BEGIN {FS = "[:=].*?#[#] "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

.PHONY: build-all
build-all: server worker
build-all:	## build all binaries

.PHONY: server
server: build-server
server:		## build server binary

.PHONY: worker
worker: build-worker
worker:		## build worker binary

.PHONY: build-%
build-%:
	@echo "Building dist/transcoderd-$*"
	@CGO_ENABLED=0 go build -o dist/transcoderd-$* $*/main.go

.PHONY: images
images: image-server image-worker
images:		## build container images

.PHONY: images
push-images: push-image-server push-image-worker
push-images:		## build and push container images

DOCKER_BUILD_ARG := --cache-to type=inline
#DOCKER_BUILD_ARG += --cache-from $(IMAGE_NAME):latest-build
#DOCKER_BUILD_ARG += --cache-from $(IMAGE_NAME):latest-base

.PHONY: image-%
.PHONY: push-image-%
image-% push-image-%: build-%
	@export DOCKER_BUILD_ARG="$(DOCKER_BUILD_ARG) $(if $(findstring push,$@),--push,--load)"; \
	docker buildx build \
	$${DOCKER_BUILD_ARG} \
	-t $(IMAGE_NAME):$(IMAGE_VERSION)-build \
	--target build \
	-f Dockerfile \
	. ; \
	docker buildx build \
		$${DOCKER_BUILD_ARG} \
		--cache-from $(IMAGE_NAME):latest-$* \
		-t $(IMAGE_NAME):$(IMAGE_VERSION)-$* \
		-f Dockerfile \
		--target $* \
		. ;

