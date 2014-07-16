NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
BIN_NAME=hostbuilder
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)-$(REVISION)

all: build
.PHONY: all

build:
	@echo "$(OK_COLOR)==> Building revision $(VERSION)...$(NO_COLOR)"
	@./script/build $(VERSION)
.PHONY: build

run: build
	@echo "$(OK_COLOR)==> Running revision $(VERSION)...$(NO_COLOR)"
	@./build/hostbuilder -config config.json
.PHONY: run

test:
	@echo "$(OK_COLOR)==> Running tests for revision $(VERSION)...$(NO_COLOR)"
	@./script/test
.PHONY: test

install-deps:
	@echo "$(OK_COLOR)==> Install dependencies...$(NO_COLOR)"
	@rm -rf .vendor
	@./script/install-deps
.PHONY: install-deps

image:
	@echo "$(OK_COLOR)==> Building Docker image...$(NO_COLOR)"
	@docker build -t registry.nitrous.io/hostbuilder .
.PHONY: image

shell: image
	@./script/shell
.PHONY: shell

test-with-docker: image
	@./script/test-with-docker
.PHONY: test-with-docker

build-with-docker: image test-with-docker
	@./script/build-with-docker
.PHONY: build-with-docker

release: image test-with-docker build-with-docker
	@./script/release $(VERSION)
.PHONY: release
