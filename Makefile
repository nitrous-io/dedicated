all: build
.PHONY: all

build:
	@./script/build
.PHONY: build

install:
	@./script/install
.PHONY: install

run: install
	@hostbuilder -config config.json
.PHONY: run

test:
	@./script/test
.PHONY: test

install-deps:
	@rm -rf .vendor
	@./script/install-deps
.PHONY: install-deps

image:
	@docker build -t registry.nitrous.io/hostbuilder .
.PHONY: image

shell: image
	@./script/shell
.PHONY: shell

test-with-docker: image
	@./script/test-with-docker
.PHONY: test-with-docker

build-with-docker: image test-with-docker
	@./script/build-with-docker
.PHONY: build-with-docker

release: image test-with-docker build-with-docker
	@./script/release
.PHONY: release
