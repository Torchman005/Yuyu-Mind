.PHONY: dev build clean test tidy

# Development
dev:
	wails dev

# Build
build:
	wails build

# Clean
clean:
	rm -rf build/dist build/bin

# Test
test:
	go test ./... -v

# Tidy dependencies
tidy:
	go mod tidy

# Run without Wails (backend only, for testing)
run-backend:
	go run ./cmd/backend/

# Format
fmt:
	go fmt ./...

# Lint
lint:
	golangci-lint run ./...
