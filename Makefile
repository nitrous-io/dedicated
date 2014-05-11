image:
	@docker build -t registry.nitrous.io/hostbuilder .
.PHONY: hostbuilder

shell: image
	@./script/shell

setup:
	@./script/setup
.PHONY: setup

run:
	@./script/install
	@hostbuilder -config config.json
.PHONY: run
