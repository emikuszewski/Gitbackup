# Makefile for git-backup

# Variables
APP_NAME := git-backup
VERSION := 1.0.0
BUILD_DIR := build
MAIN := main.go
GOARCH := arm64
GOOS := darwin

# Default build target - macOS ARM64 (Apple Silicon)
.PHONY: build
build: clean
	@echo "Building $(APP_NAME) for macOS ARM64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)"

# Build for current platform
.PHONY: build-current
build-current: clean
	@echo "Building $(APP_NAME) for current platform..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN)
	@echo "Build complete: $(BUILD_DIR)/$(APP_NAME)"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning up..."
	@rm -rf $(BUILD_DIR)

# Run tests
.PHONY: test
test:
	go test -v ./...

# Install dependencies
.PHONY: deps
deps:
	go mod download

# Helper target to print version
.PHONY: version
version:
	@echo "$(APP_NAME) version $(VERSION)"

# Help command
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build            - Build for macOS ARM64 (Apple Silicon)"
	@echo "  build-current    - Build for the current platform"
	@echo "  clean            - Remove build artifacts"
	@echo "  test             - Run tests"
	@echo "  deps             - Install dependencies"
	@echo "  version          - Print version information"
	@echo "  help             - Show this help message"
