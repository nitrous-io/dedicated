NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
APP=nitro
BUILD_DIR=./build
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)-$(REVISION)

all: build

build: clean
	@echo "$(OK_COLOR)==> Building revision $(VERSION)...$(NO_COLOR)"
	@script/build $(VERSION)

clean:
	rm -rf .vendor/bin/$(APP)

image:
	@script/image $(APP) $(VERSION)

ci: image
	@script/ci $(APP) $(VERSION)

release: ci
	@script/release $(APP) $(VERSION)

format:
	go fmt ./...

test:
	@echo "$(OK_COLOR)==> Testing...$(NO_COLOR)"
	@script/test $(TEST)

xcompile:
	@echo "$(OK_COLOR)==> Local cross compilation for revision $(VERSION)...$(NO_COLOR)"
	@script/local-xcompile $(VERSION)
.PHONY: xcompile

xcompile-docker: image
	@echo "$(OK_COLOR)==> Cross compilation in docker for revision $(VERSION)...$(NO_COLOR)"
	@script/docker-run-xcompile $(APP) $(VERSION)
.PHONY: xcompile-docker

xrelease: xcompile-docker
	@echo "$(OK_COLOR)==> Packaging revision $(VERSION)...$(NO_COLOR)"
	@script/multi-release $(APP) $(VERSION)
.PHONY: xrelease

publish: xrelease
	@echo "$(OK_COLOR)==> Uploading binaries for revision $(VERSION)...$(NO_COLOR)"
	@script/publish $(APP) $(VERSION)
.PHONY: publish 

.PHONY: all build clean format test image ci
