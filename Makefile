.PHONY: all build install lint test test-short quality check

all: lint test build

build:
	go build ./...

install:
	go install ./cmd/goquality

lint:
	go vet ./...
	@out=$$(gofmt -l cmd internal | grep -v testdata); if [ -n "$$out" ]; then echo "not gofmt-ed:"; echo "$$out"; exit 1; fi

test:
	go test -race ./...

# Skips tests that need network access (govulncheck).
test-short:
	go test -short ./...

# Run goquality on itself.
quality:
	go run ./cmd/goquality --min-score 95

# Compare with the merge base of origin's default branch; fail on regressions.
check:
	go run ./cmd/goquality check
