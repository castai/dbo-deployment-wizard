.PHONY: lint fix gen test tidy build

GORELEASER := go run github.com/goreleaser/goreleaser/v2@v2.18.2

# Lint the code.
lint:
	golangci-lint run

# Lint, auto-fix fixable issues, and validate the release config.
fix:
	golangci-lint run --fix
	$(GORELEASER) check

# Regenerate mocks (configured in .mockery.yaml).
gen:
	mockery

# Run the tests.
test:
	go test ./...

tidy:
	go mod tidy

# Build a local snapshot: binaries, nothing published.
build:
	$(GORELEASER) release --snapshot --clean
