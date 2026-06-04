.PHONY: build test test-coverage coverage-html coverage-func clean help

# Enable CGO for DuckDB bindings
export CGO_ENABLED=1

# Default target
build:
	CGO_ENABLED=1 go build -o tmoney .

help:
	@echo "Available targets:"
	@echo "  build          - Build the tmoney executable"
	@echo "  test           - Run all tests"
	@echo "  test-coverage  - Run tests with coverage and show summary"
	@echo "  coverage-html  - Generate HTML coverage report (opens in browser)"
	@echo "  coverage-func  - Show function-level coverage breakdown"
	@echo "  clean          - Remove coverage files and built executable"

# Run all tests (-p 8: packages run in parallel; each test isolates its own DB/config, so this is safe)
test:
	CGO_ENABLED=1 go test -p 8 ./...

# Run tests with coverage and display summary
test-coverage:
	CGO_ENABLED=1 go test -p 8 -coverprofile=coverage.out -covermode=atomic ./...
	@echo ""
	@echo "Coverage summary:"
	@go tool cover -func=coverage.out | tail -1

# Generate HTML coverage report
coverage-html: test-coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@command -v open >/dev/null 2>&1 && open coverage.html || true

# Show function-level coverage
coverage-func: test-coverage
	go tool cover -func=coverage.out

# Clean up coverage files and built executable
clean:
	rm -f coverage.out coverage.html tmoney
