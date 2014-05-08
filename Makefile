setup:
	@./script/setup

.PHONY: setup

dev:
	@source ./script/env

.PHONY: run

run:
	@./script/install
	@hostbuilder -config config.json

.PHONY: run
