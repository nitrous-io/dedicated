NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
BIN_NAME=nitrousd
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)-$(REVISION)

all: build

build:
	@echo "$(OK_COLOR)==> Building revision $(VERSION)...$(NO_COLOR)"
	@script/build

format:
	go fmt ./...

run: build
	@echo "$(OK_COLOR)==> Running...$(NO_COLOR)"
	@build/$(BIN_NAME)

.PHONY: all build format run
