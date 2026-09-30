BIN     := asp
PREFIX  := $(HOME)/.local/bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install test fmt vet

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) .

# Writes only $(PREFIX)/asp; that directory also holds the claude launcher.
install: build
	install -Dm755 $(BIN) $(PREFIX)/$(BIN)

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...
