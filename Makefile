.PHONY: lint fix gen test tidy

# Lint the code.
lint:
	golangci-lint run

# Lint and auto-fix fixable issues.
fix:
	golangci-lint run --fix

# Regenerate mocks (configured in .mockery.yaml).
gen:
	mockery

# Run the tests.
test:
	go test ./...

tidy:
	go mod tidy
