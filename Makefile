.PHONY: build install uninstall test test-coverage coverage-html coverage-func clean help

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

# Install destination on macOS/Linux, following the GNU convention so it can be
# redirected without editing this file:
#   make install PREFIX=$HOME/.local   - user-local install, no sudo needed
#   make install BINDIR=/opt/bin       - pick the bin directory outright
#   make install DESTDIR=/tmp/stage    - staged install for packaging (deb/rpm)
# Windows ignores these; see the install target below for why.
PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
INSTALL ?= install

# Default target
build:
	go build -o $(BINARY) .

ifeq ($(OS),Windows_NT)

# Windows has no /usr/local, and the per-user "Programs" folder
# (%LOCALAPPDATA%\Programs) is a GUI-app convention that is never on PATH. The
# right home for a Go CLI is GOBIN — %USERPROFILE%\go\bin by default, which the
# official Go MSI already appends to the *user* PATH and where the rest of the
# Go tooling (gopls, dlv, k9s) lands. It needs no elevation.
#
# So the Windows recipes delegate to the Go toolchain rather than hand-rolling
# cmd.exe mkdir/copy/del. `go install` builds and places the binary itself (no
# `build` prerequisite, and it ignores -o, so $(BINARY) is not used); `go clean
# -i` removes exactly what it created. Both are shell-agnostic, so they work
# under cmd.exe and under MSYS2/Git Bash, where $(OS) is also Windows_NT but
# the recipe shell is sh.
install:
	go install .
	@echo Installed $(BINARY) to GOBIN, or %USERPROFILE%\go\bin when GOBIN is unset.
	@echo If the tmoney command is not found, add that directory to your PATH.

uninstall:
	go clean -i .
	@echo Removed the installed $(BINARY).

else

# `install -d` is deliberately skipped when the directory already exists: on an
# existing directory it re-chmods to 0755 (relaxing a private ~/.local/bin from
# 0700) and, when that chmod is denied, prints an alarming error while still
# exiting 0 — so it would neither gate the install nor fail usefully.
install: build
	@test -d "$(DESTDIR)$(BINDIR)" || $(INSTALL) -d "$(DESTDIR)$(BINDIR)" || \
		{ echo "error: cannot create $(DESTDIR)$(BINDIR)"; exit 1; }
	@test -w "$(DESTDIR)$(BINDIR)" || { \
		echo "error: $(DESTDIR)$(BINDIR) is not writable. Either:"; \
		echo "  install where you already have write access:  make install PREFIX=\$$HOME/.local"; \
		echo "  or install system-wide:                       sudo make install"; \
		echo "    (note: sudo re-runs 'go build' as root, leaving a root-owned ./$(BINARY))"; \
		exit 1; }
	$(INSTALL) -m 0755 "$(BINARY)" "$(DESTDIR)$(BINDIR)/$(BINARY)"
	@echo "Installed $(DESTDIR)$(BINDIR)/$(BINARY)"

uninstall:
	rm -f "$(DESTDIR)$(BINDIR)/$(BINARY)"
	@echo "Removed $(DESTDIR)$(BINDIR)/$(BINARY)"

endif

help:
	@echo "Available targets:"
	@echo "  build          - Build the tmoney executable"
	@echo "  install        - Install tmoney (macOS/Linux: $(BINDIR); Windows: GOBIN)"
	@echo "  uninstall      - Remove the installed tmoney binary"
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
