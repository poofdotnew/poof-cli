BINARY_NAME = poof
MODULE       = github.com/poofdotnew/poof-cli
VERSION_PKG  = $(MODULE)/internal/version

VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  = $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    = $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS = -ldflags "\
  -X $(VERSION_PKG).Version=$(VERSION) \
  -X $(VERSION_PKG).Commit=$(COMMIT) \
  -X $(VERSION_PKG).Date=$(DATE) \
  -s -w"

.PHONY: build install clean test fmt vet lint all release

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/poof

install:
	go install $(LDFLAGS) ./cmd/poof

clean:
	rm -rf bin/ dist/

test:
	go test ./... -count=1

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

# Cross-compile for all major platforms
release: clean
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64   ./cmd/poof
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64   ./cmd/poof
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64    ./cmd/poof
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64    ./cmd/poof
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/poof
	@echo "Binaries in dist/"
	@ls -lh dist/

all: lint test build
