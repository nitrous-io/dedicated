image:
	@docker build -t registry.nitrous.io/compass .
.PHONY: image

shell: image
	@./script/shell
.PHONY: shell

# setup a local development environment
setup:
	@go get github.com/nitrous-io/goop
	@goop install
.PHONY: setup

run: build
	@./build/compass
.PHONY: setup

test: image
	@./script/test
.PHONY: test

build: image test
	@./script/build
.PHONY: build

release: build
	@./script/release
