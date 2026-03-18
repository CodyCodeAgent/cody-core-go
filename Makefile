.PHONY: test test-race lint vet fmt tidy cover clean help

## help: Show this help message
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':'

## test: Run all tests
test:
	go test ./...

## test-race: Run all tests with race detector
test-race:
	go test -race ./...

## test-cover: Run tests with coverage report
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view HTML report: go tool cover -html=coverage.out"

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## vet: Run go vet
vet:
	go vet ./...

## fmt: Format code
fmt:
	gofmt -s -w .
	goimports -w .

## tidy: Tidy go modules
tidy:
	go mod tidy

## check: Run all checks (vet + lint + test)
check: vet lint test-race

## clean: Remove build artifacts
clean:
	rm -f coverage.out coverage.html
	go clean -testcache
