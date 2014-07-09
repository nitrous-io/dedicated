build:
	@./script/build
.PHONY: build

run: build
	@goop exec forego start
.PHONY: run
