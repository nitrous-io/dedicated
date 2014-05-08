setup:
	@./script/setup

.PHONY: setup

dev:
	@source ./script/env

.PHONY: run

run:
	@go get ./...
	@hostbuilder -config config.json

.PHONY: run
