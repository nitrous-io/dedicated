run:
	@go get ./...
	@hostbuilder -config config.json

.PHONY: run
