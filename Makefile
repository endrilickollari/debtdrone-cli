.PHONY: all build test clean snapshot help

BINARY_NAME=debtdrone
DIST_DIR=dist
CLI_PATH=./cmd/debtdrone

help:
	@echo "DebtDrone CLI - Build Commands"
	@echo ""
	@echo "  make build      - Build the CLI binary locally"
	@echo "  make test       - Run all tests"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make snapshot   - Create a snapshot release (no push)"
	@echo ""

all: clean test build

build:
	@echo "🚧 Building..."
	@go build -o $(DIST_DIR)/$(BINARY_NAME) $(CLI_PATH)
	@echo "✅ Built to $(DIST_DIR)/$(BINARY_NAME)"

test:
	@echo "🧪 Running tests..."
	@go test ./internal/...
	@echo "✅ Tests completed"

clean:
	@echo "🧹 Cleaning build artifacts..."
	@rm -rf $(DIST_DIR)
	@echo "✅ Clean complete"

snapshot:
	@echo "📦 Building snapshot with Docker (CGO cross-compilation)..."
	docker run --rm --privileged \
		-v $(PWD):/code \
		-w /code \
		ghcr.io/goreleaser/goreleaser-cross:v1.23.2 \
		release --snapshot --clean
	@echo "✅ Snapshot created in dist/"

install: build
	@echo "📦 Installing $(BINARY_NAME)..."
	@cp $(DIST_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "✅ Installed to /usr/local/bin/$(BINARY_NAME)"

uninstall:
	@echo "🗑️  Uninstalling $(BINARY_NAME)..."
	@rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "✅ Uninstalled"
