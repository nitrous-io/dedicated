image:
	@./script/docker-image
.PHONY: image

build:
	@./script/build
.PHONY: build

run: build
	@goop exec forego start
.PHONY: run

test: image
	@./script/docker-test
.PHONY: run

shell: image
	@./script/docker-shell
.PHONY: shell
