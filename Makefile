NO_COLOR = \033[0m
OK_COLOR = \033[32;01m
ERROR_COLOR = \033[31;01m
WARN_COLOR = \033[33;01m
APP = tender 
REVISION = $(shell git rev-parse --short HEAD)
BASE_VERSION = $(shell cat VERSION)
VERSION = $(BASE_VERSION)+$(BUILD_NUMBER)-$(REVISION)

image:
	@echo "$(OK_COLOR)==> Building Docker image...$(NO_COLOR)"
	@./script/image $(APP) $(VERSION)

build:
	@echo "$(OK_COLOR)==> Building revision $(VERSION)...$(NO_COLOR)"
	@./script/build $(APP) $(VERSION)

run: build
	@echo "$(OK_COLOR)==> Running revision $(VERSION)...$(NO_COLOR)"
	@goop exec forego start

test: 
	@echo "$(OK_COLOR)==> Running tests for revision $(VERSION)...$(NO_COLOR)"
	@./script/test

ci: image
	@echo "$(OK_COLOR)==> Running ci (testing and building within Docker)...$(NO_COLOR)"
	@./script/ci $(APP) $(VERSION)

release: ci
	@echo "$(OK_COLOR)==> Releasing tender binary archive version $(VERSION)...$(NO_COLOR)"
	@./script/wrap-release-docker $(APP) $(VERSION)

shell: image
	@./script/docker-shell

all: build

.PHONY: all shell test run build image ci release
