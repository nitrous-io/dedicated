NO_COLOR=\033[0m
OK_COLOR=\033[32;01m
ERROR_COLOR=\033[31;01m
WARN_COLOR=\033[33;01m
BIN_NAME=freighterd

all:
	@mkdir -p bin/
	@echo "$(OK_COLOR)==> Installing dependencies$(NO_COLOR)"
	go get -d -v ./...
	@echo "$(OK_COLOR)==> Building$(NO_COLOR)"
	go install -x ./...
	cp $(GOPATH)/bin/$(BIN_NAME) bin/$(BIN_NAME)

format:
	go fmt ./...

test:
	@echo "$(OK_COLOR)==> Testing...$(NO_COLOR)"
	@go list -f '{{range .TestImports}}{{.}} {{end}}' ./... | xargs -n1 go get -d
	go test -test.v ./...

run: all
	@echo "$(OK_COLOR)==> Running...$(NO_COLOR)"
	@bin/freighterd

.PHONY: all format test run
