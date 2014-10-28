NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
COMMIT=$(shell git rev-parse --short HEAD)

APP=all
SUITE=

all: binary

binary:
	@./script/build all
.PHONY: binary

nitro:
	@./script/build nitro
.PHONY: nitro

freighter:
	@./script/build freighter
.PHONY: freighter

hostbuilder:
	@./script/build hostbuilder
.PHONY: hostbuilder

nitrous:
	@./script/build nitrous
.PHONY: nitrous

compass:
	@./script/build compass
.PHONY: compass

tender:
	@./script/build tender
.PHONY: tender

test:
	@echo "$(OK_COLOR)==> Running tests $(COMMIT)...$(NO_COLOR)"
	@./script/test $(APP) $(SUITE)
.PHONY: test

release: ci-build
	@echo "$(OK_COLOR)==> Releasing $(APP) $(COMMIT)...$(NO_COLOR)"
	@./script/release $(APP) true
.PHONY: release

ci-test: image
	@echo "$(OK_COLOR)==> Running tests in CI mode $(COMMIT)...$(NO_COLOR)"
	@./script/ci $(APP) $(COMMIT)
.PHONY: ci-test

ci-build: image
	@./script/build $(APP) true
.PHONY: ci-build

run-dev: binary
	@godep go run ./script/dev.go
.PHONY: run-dev

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
	@./script/image $(COMMIT)
.PHONY: image

createdb:
	createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_development || true
	sudo su postgres -c "psql -d nitrous_development -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" || true
	createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_test || true
	sudo su postgres -c "psql -d nitrous_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" || true
	createdb -E UTF8 -T template0 -l en_US.utf8 tender_test || true
	sudo su postgres -c "psql -d tender_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" || true
	createdb -E UTF8 -T template0 -l en_US.utf8 compass_test || true
	sudo su postgres -c "psql -d compass_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" || true

dropdb:
	dropdb --if-exists nitrous_development
	dropdb --if-exists nitrous_migrations_test
	dropdb --if-exists nitrous_test
	dropdb --if-exists tender_test
	dropdb --if-exists compass_test
