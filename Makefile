# Makefile for toggl CLI

BINARY_NAME = toggl
CMD_PATH = ./src
BUILD_DIR = ./bin
INSTALL_DIR = $(HOME)/.local/bin

GIT_COUNT := $(shell git rev-list --count HEAD)
GIT_HASH  := $(shell git rev-parse --short HEAD)
VERSION   := $(GIT_COUNT)-h$(GIT_HASH)
LDFLAGS   := -ldflags "-X main.version=$(VERSION)"

# Default: build local binary
.PHONY: all
all: build install

# Build local binary
.PHONY: build
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

# Install to user-local bin directory
.PHONY: install
install: build
	@echo "Installing $(BINARY_NAME) to $(INSTALL_DIR)..."
	@mkdir -p $(INSTALL_DIR)
	cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)/*

# Run the CLI locally (without installing)
.PHONY: run
run:
	$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

# Cross-compile for multiple OS/ARCH
.PHONY: cross
cross:
	@echo "Building cross-platform binaries..."
	mkdir -p $(BUILD_DIR)/release
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY_NAME)-linux   $(CMD_PATH)
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY_NAME)-mac     $(CMD_PATH)
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY_NAME)-mac-arm $(CMD_PATH)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/release/$(BINARY_NAME).exe    $(CMD_PATH)

# Format Go code
.PHONY: fmt
fmt:
	go fmt ./...

# Run tests
.PHONY: test
test:
	go test ./...
