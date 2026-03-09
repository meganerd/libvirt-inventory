BINARY := lvi
MODULE := github.com/meganerd/libvirt-inventory
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build install clean test

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/lvi

install:
	go install $(LDFLAGS) ./cmd/lvi

clean:
	rm -f $(BINARY)

test:
	go test ./...
