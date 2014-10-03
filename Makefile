NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)
VERSION=$(BASE_VERSION)+$(BUILD_NUMBER)-$(REVISION)
PROJECTS=nitrous nitro hostbuilder freighter

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

assets:
	@./script/build-assets
.PHONY: assets

test-nitrous:
	@echo "$(OK_COLOR)==> Running tests for nitrous revision $(VERSION)...$(NO_COLOR)"
	$(MAKE) -C nitrous test
.PHONY: test-nitrous

test-nitro:
	@echo "$(OK_COLOR)==> Running tests for nitro revision $(VERSION)...$(NO_COLOR)"
	$(MAKE) -C nitro test
.PHONY: test-nitro

test-hostbuilder:
	@echo "$(OK_COLOR)==> Running tests for hostbuilder revision $(VERSION)...$(NO_COLOR)"
	$(MAKE) -C hostbuilder test
.PHONY: test-hostbuilder

test-freighter:
	@echo "$(OK_COLOR)==> Running tests for freighter revision $(VERSION)...$(NO_COLOR)"
	$(MAKE) -C freighter test
.PHONY: test-freighter

test: test-nitrous test-nitro test-hostbuilder test-freighter
.PHONY: test
