.DEFAULT_GOAL := help
.PHONY: build lint fmt test clean tidy vuln modernize-check audit check-release test/cover help

BINARY := ogen-mcp

## build: compile the binary
build:
	go build -trimpath -o $(BINARY) ./cmd/ogen-mcp

## lint: run golangci-lint
lint:
	golangci-lint run --new-from-rev="origin/main" ./...

## fmt: format code
fmt:
	golangci-lint fmt ./...

## test: run tests with race detector
test:
	go test -race -shuffle=on -coverprofile=coverage.out -covermode=atomic ./...

## clean: remove build artifacts
clean:
	rm -f $(BINARY) coverage.out

## tidy: tidy go modules
tidy:
	go mod tidy

## vuln: run govulncheck
vuln:
	go tool govulncheck ./...

## modernize-check: preview Go modernizations without changing files
modernize-check:
	go fix -n ./...

## audit: run all checks
audit: lint test vuln modernize-check
	go mod tidy -diff
	go mod verify

## check-release: validate goreleaser config
check-release:
	goreleaser check -q

## test/cover: open HTML coverage report
test/cover: test
	go tool cover -html=coverage.out

## help: show this help
help:
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'
