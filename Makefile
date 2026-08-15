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

# Run the full Wails app from source.
# Requires frontend/dist to exist first (run: cd frontend && npm run build).
run-backend:
	go run .

# Format
fmt:
	go fmt ./...

# Lint
lint:
	golangci-lint run ./...
