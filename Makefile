NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
VERSION=$(shell git rev-parse --short HEAD)

APP=all
SUITE=

all: binary

binary:
	@ENV=$(ENV) ./script/build all
.PHONY: binary

freighter:
	@./script/build freighter
.PHONY: freighter

hostbuilder:
	@./script/build hostbuilder
.PHONY: hostbuilder

nitrous:
	./script/build nitrous
.PHONY: nitrous

nitrous-dev:
	ENV=development ./script/build nitrous
.PHONY: nitrous-dev

compass:
	@./script/build compass
.PHONY: compass

tender:
	@./script/build tender
.PHONY: tender

test: image
	@echo "$(OK_COLOR)==> Running tests $(VERSION)...$(NO_COLOR)"
	@./script/test-in-container $(APP) $(VERSION) $(SUITE)
.PHONY: test

shell: image
	@./script/test-in-container shell
.PHONY: shell

release: ci-build
	@echo "$(OK_COLOR)==> Releasing $(APP) $(VERSION)...$(NO_COLOR)"
	@./script/release $(APP) true
.PHONY: release

ci-build: image
	@./script/build $(APP) true
.PHONY: ci-build

run-dev: ENV := development
run-dev: binary
	godep go run ./script/dev.go
.PHONY: run-dev

run-nitrous: nitrous
	@forego start -f nitrous/Procfile
.PHONY: run-nitrous

run-nitrous-dev: nitrous-dev
	@forego start -f nitrous/Procfile.dev -e nitrous/.env
.PHONY: run-nitrous-dev

run-hostbuilder: hostbuilder
	@./build/hostbuilder -config hostbuilder/config.json
.PHONY: run-hostbuilder

image:
	@./script/image
.PHONY: image

createdb:
	createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_development || true
	sudo su postgres -c "psql -d nitrous_development -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE EXTENSION IF NOT EXISTS citext'" || true
	createdb -E UTF8 -T template0 -l en_US.utf8 nitrous_test || true
	sudo su postgres -c "psql -d nitrous_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto; CREATE EXTENSION IF NOT EXISTS citext'" || true
	createdb -E UTF8 -T template0 -l en_US.utf8 tender_test || true
	sudo su postgres -c "psql -d tender_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" || true
	createdb -E UTF8 -T template0 -l en_US.utf8 compass_test || true
	sudo su postgres -c "psql -d compass_test -c 'CREATE EXTENSION IF NOT EXISTS hstore; CREATE EXTENSION IF NOT EXISTS pgcrypto;'" || true
.PHONY: createdb

dropdb:
	dropdb --if-exists nitrous_development
	dropdb --if-exists nitrous_migrations_test
	dropdb --if-exists nitrous_test
	dropdb --if-exists tender_test
	dropdb --if-exists compass_test
.PHONY: dropdb

dbseed: nitrous
	forego run -e nitrous/.env build/dbseed
.PHONY: dbseed
