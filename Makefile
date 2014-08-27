NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
APP=nitro
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)-$(REVISION)

all: build

build: clean
	@echo "$(OK_COLOR)==> Building revision $(VERSION)...$(NO_COLOR)"
	@script/build $(APP) $(VERSION)

xbuild:
	@echo "$(OK_COLOR)==> Cross-Compiling revision $(VERSION)...$(NO_COLOR)"
	@script/xbuild $(APP) $(VERSION)

clean:
	rm -f .vendor/bin/$(APP)

image:
	@script/image $(APP) $(VERSION)

ci: image
	@script/ci $(APP) $(VERSION)

format:
	go fmt ./...

test:
	@echo "$(OK_COLOR)==> Testing...$(NO_COLOR)"
	@script/test $(TEST)

release: ci
	@echo "$(OK_COLOR)==> Packaging revision $(VERSION)...$(NO_COLOR)"
	@script/wrap-release-docker $(APP) $(VERSION)
.PHONY: release

publish: release
	@echo "$(OK_COLOR)==> Uploading binaries for revision $(VERSION)...$(NO_COLOR)"
	@script/publish $(APP) $(VERSION)
.PHONY: publish

.PHONY: all build clean format test image ci
