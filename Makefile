NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
BIN_NAME=freighterd
VERSION=$(shell git rev-parse --short HEAD)

all: build

build:
	@echo "$(OK_COLOR)==> Building revision $(VERSION)...$(NO_COLOR)"
	@script/build $(VERSION)

format:
	go fmt ./...

test:
	@echo "$(OK_COLOR)==> Testing...$(NO_COLOR)"
	@script/test

run: build
	@echo "$(OK_COLOR)==> Running...$(NO_COLOR)"
	@build/$(BIN_NAME)

.PHONY: all build format test run

all: build
