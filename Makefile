BINARY := firstspark
CMD := ./cmd/firstspark
BIN_DIR := bin
TEST_DIR := test

GO ?= go
GOFLAGS ?=

# Installation prefix and staging directory. Install into a staging tree with
# `make install DESTDIR=/tmp/stage`; override the prefix with PREFIX=~/.local.
PREFIX ?= /usr/local
DESTDIR ?=
BINDIR ?= $(PREFIX)/bin
MANDIR ?= $(PREFIX)/share/man/man1

.PHONY: all build build/gui build/headless gui run install install/headless uninstall test test/race fuzz lint fmt vet fixtures clean tidy

# How long each fuzz target runs under `make fuzz`.
FUZZTIME ?= 15s

# `make` builds the runnable GUI. On a machine without CGO or the
# OpenGL/X11 headers, use `make build/headless` instead.
all: build/gui

build: build/gui

# Build the Fyne GUI. Requires CGO plus the OpenGL and X11/Wayland headers:
#   sudo apt install libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev
build/gui:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 $(GO) build $(GOFLAGS) -tags gui -o $(BIN_DIR)/$(BINARY) $(CMD)

# Headless build: no CGO, no graphics libraries. Auto Assembler custom types run
# through the pure-Go interpreter (pkg/aaexec) instead of the JIT.
build/headless:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(BINARY) $(CMD)

# Alias for `make build/gui`.
gui: build/gui

run: build/gui
	$(BIN_DIR)/$(BINARY)

# Install the GUI binary and its manual page. Override PREFIX and DESTDIR as
# needed, e.g. `sudo make install` or `make install PREFIX=$(HOME)/.local`.
# `make install/headless` installs the no-CGO build instead.
install: build/gui
install/headless: build/headless
install install/headless:
	install -d $(DESTDIR)$(BINDIR) $(DESTDIR)$(MANDIR)
	install -m 0755 $(BIN_DIR)/$(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -m 0644 docs/$(BINARY).1 $(DESTDIR)$(MANDIR)/$(BINARY).1

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY) $(DESTDIR)$(MANDIR)/$(BINARY).1

test:
	$(GO) test $(GOFLAGS) ./...

# Run the tests under the race detector.
test/race:
	$(GO) test $(GOFLAGS) -race ./...

# Fuzz every FuzzXxx target in pkg/... for FUZZTIME each. `go test -fuzz`
# accepts only one target at a time, so iterate over them.
fuzz:
	@for pkg in $$($(GO) list ./pkg/...); do \
		for target in $$($(GO) test -list '^Fuzz' $$pkg 2>/dev/null | grep '^Fuzz'); do \
			echo "==> $$target ($$pkg)"; \
			$(GO) test -run '^$$' -fuzz "^$$target$$" -fuzztime $(FUZZTIME) $$pkg || exit 1; \
		done; \
	done

lint: fmt vet

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet $(GOFLAGS) ./...
	$(GO) vet $(GOFLAGS) -tags gui ./...

tidy:
	$(GO) mod tidy

# Build the C fixtures used by the integration tests.
fixtures:
	$(MAKE) -C $(TEST_DIR) all

clean:
	rm -rf $(BIN_DIR)
	$(MAKE) -C $(TEST_DIR) clean
