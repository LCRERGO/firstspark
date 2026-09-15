BINARY := firstspark
CMD := ./cmd/firstspark
BIN_DIR := bin
TEST_DIR := test

GO ?= go
GOFLAGS ?=

.PHONY: all build build/gui build/headless gui run test lint fmt vet fixtures clean tidy

# `make` builds the runnable GUI. On a machine without CGO or the
# OpenGL/X11 headers, use `make build/headless` instead.
all: build/gui

build: build/gui

# Build the Fyne GUI. Requires CGO plus the OpenGL and X11/Wayland headers:
#   sudo apt install libgl1-mesa-dev xorg-dev libwayland-dev libxkbcommon-dev
build/gui:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 $(GO) build $(GOFLAGS) -tags gui -o $(BIN_DIR)/$(BINARY) $(CMD)

# Headless build: no CGO, no graphics libraries (Auto Assembler custom types
# are unavailable in this build; Lua types still work).
build/headless:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(BINARY) $(CMD)

# Alias for `make build/gui`.
gui: build/gui

run: build/gui
	$(BIN_DIR)/$(BINARY)

test:
	$(GO) test $(GOFLAGS) ./...

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
