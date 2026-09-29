.PHONY: lint fix gen test tidy build

GORELEASER := go run github.com/goreleaser/goreleaser/v2@v2.18.2
# use same one as CI does
GOLANGCI_LINT_VERSION = $(shell yq '.env.GOLANGCI_LINT_VERSION' .github/workflows/ci.yaml)
LINTER = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

# Lint the code.
lint:
	$(LINTER) run

# Lint, auto-fix fixable issues, and validate the release config.
fix:
	go fmt ./...
	$(LINTER) run --fix
	$(GORELEASER) check

gen:
	go generate ./...

# Run the tests.
test:
	go test ./...

tidy:
	go mod tidy

# Build a local snapshot: binaries, nothing published.
build:
	$(GORELEASER) release --snapshot --clean
