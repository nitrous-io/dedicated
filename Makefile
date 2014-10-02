NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)+$(BUILD_NUMBER)-$(REVISION)

all: nitro freighter hostbuilder nitrous

nitro:
	@./script/build nitro nitro $(VERSION)
.PHONY: nitro

freighter:
	@./script/build freighter/freighterd freighterd $(VERSION)
.PHONY: freighter

hostbuilder:
	@./script/build hostbuilder/hostbuilder hostbuilder $(VERSION)
.PHONY: hostbuilder

nitrous:
	@./script/build nitrous/nitrousd nitrousd $(VERSION)
	@./script/build nitrous/cmd/... dbseed $(VERSION)
	@./script/build nitrous/cmd/... hssh $(VERSION)
	@./script/build nitrous/cmd/... adminuserseed $(VERSION)
.PHONY: nitrous
