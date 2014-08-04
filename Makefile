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
	@./script/install-deps
.PHONY: install-deps

image:
	@echo "$(OK_COLOR)==> Building Docker image...$(NO_COLOR)"
	@./script/image $(BIN_NAME) $(VERSION)
.PHONY: image

ci: image
	@echo "$(OK_COLOR)==> Running tests within Docker...$(NO_COLOR)"
	@./script/ci $(BIN_NAME) $(VERSION)
.PHONY: ci

shell: image
	@./script/shell
.PHONY: shell

build-with-docker: image ci
	@./script/build-with-docker
.PHONY: build-with-docker

release: build-with-docker
	@./script/release $(VERSION)
.PHONY: release
