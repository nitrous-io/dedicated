NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)+$(BUILD_NUMBER)-$(REVISION)

all: nitro

nitro:
	@./script/build nitro nitro $(VERSION)
.PHONY: nitro
