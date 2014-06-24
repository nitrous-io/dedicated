image:
	@docker build -t registry.nitrous.io/hostbuilder .
.PHONY: image

shell: image
	@./script/shell
.PHONY: shell

deps:
	@rm -rf .vendor
	@./script/install-deps
.PHONY: deps

run:
	@./script/install
	@hostbuilder -config config.json
.PHONY: run

test: image
	@./script/test
.PHONY: test

build: image test
	@./script/build
.PHONY: build

release: image test build
	@./script/release
.PHONY: release
