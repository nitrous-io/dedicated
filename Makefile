NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
BIN_NAME=freighterd

image:
	@docker build -t registry.nitrous.io/freighter .
.PHONY: image

deps:
	@./script/install-deps
.PHONY: image

build: image
	@./script/build
.PHONY: build

shell: image
	@./script/shell
.PHONY: shell

test: image
	@./script/test
.PHONY: test

format:
	go fmt ./...
.PHONY: format

run: build
	@echo "$(OK_COLOR)==> Running...$(NO_COLOR)"
	@build/freighterd
.PHONY: run
