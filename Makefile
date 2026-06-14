.PHONY: build test test-coverage coverage-html coverage-func clean help

# Enable CGO for DuckDB bindings (required on macOS, Linux, and Windows).
# `export` puts it in every recipe shell's environment, so recipes don't need
# an inline `CGO_ENABLED=1 ...` prefix — that POSIX syntax fails under the
# cmd.exe shell GNU make uses on Windows.
export CGO_ENABLED=1

# Output binary name. An explicit `-o` target gets no automatic extension, so
# Windows needs the `.exe` itself — which also keeps it under the *.exe
# .gitignore rule; macOS/Linux use a bare name.
ifeq ($(OS),Windows_NT)
BINARY := tmoney.exe
else
BINARY := tmoney
endif

# Default target
build:
	go build -o $(BINARY) .

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
	go test -p 8 ./...

# Run tests with coverage and display summary
test-coverage:
	go test -p 8 -coverprofile=coverage.out -covermode=atomic ./...
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
	rm -f coverage.out coverage.html tmoney tmoney.exe
