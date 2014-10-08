NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
REVISION=$(shell git rev-parse --short HEAD)
BASE_VERSION=$(shell cat VERSION)

BUILD_NUMBER ?= 0
VERSION=$(BASE_VERSION)+$(BUILD_NUMBER)-$(REVISION)

APP=all

all: binary

binary:
	@./script/build all $(VERSION)
.PHONY: binary

nitro:
	@./script/build nitro $(VERSION)
.PHONY: nitro

freighter:
	@./script/build freighter $(VERSION)
.PHONY: freighter

hostbuilder:
	@./script/build hostbuilder $(VERSION)
.PHONY: hostbuilder

nitrous:
	@./script/build nitrous $(VERSION)
.PHONY: nitrous

test:
	@echo "$(OK_COLOR)==> Running tests $(VERSION)...$(NO_COLOR)"
	@./script/test $(APP)
.PHONY: test

release: ci-build
	@echo "$(OK_COLOR)==> Releasing $(APP) $(VERSION)...$(NO_COLOR)"
	@./script/release $(APP) $(VERSION) true
.PHONY: test

ci-test: image
	@echo "$(OK_COLOR)==> Running tests in CI mode $(VERSION)...$(NO_COLOR)"
	@./script/ci $(APP)
.PHONY: ci-test

ci-build: image
	@./script/build $(APP) $(VERSION) true
.PHONY: ci-build

run-nitrous: nitrous
	@forego start -f nitrous/Procfile
.PHONY: run-nitrous

run-nitrous-dev: nitrous
	@forego start -f nitrous/Procfile.dev
.PHONY: run-nitrous

run-hostbuilder: hostbuilder
	@./build/hostbuilder -config hostbuilder/config.json
.PHONY: run-hostbuilder

image:
	@./script/image $(REVISION)
.PHONY: image

createdb:
	createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_development || true
	sudo su postgres -c "psql -d nitrous_development -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" || true
	createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_test || true
	sudo su postgres -c "psql -d nitrous_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" || true

dropdb:
	dropdb --if-exists nitrous_development
	dropdb --if-exists nitrous_migrations_test
	dropdb --if-exists nitrous_test
