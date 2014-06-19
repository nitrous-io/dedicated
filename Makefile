NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
BIN_NAME=nitro
BUILD_DIR=./build
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)-$(REVISION)

all: build

build: clean
	@echo "$(OK_COLOR)==> Building revision $(VERSION)...$(NO_COLOR)"
	GOBIN=$(BUILD_DIR) goop go install -ldflags "-X main.version $(VERSION)" -x -v .

clean:
	rm -rf $(BUILD_DIR)

format:
	go fmt ./...

test:
	@echo "$(OK_COLOR)==> Testing...$(NO_COLOR)"
	goop exec ginkgo -r -trace -keepGoing

.PHONY: all build clean format test
