.PHONY: build test dist publish clean

GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf dev)

build:
	mkdir -p bin
	$(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/aissh ./cmd/aissh

test:
	$(GO) test ./...

dist:
	GO="$(GO)" ./scripts/build-release.sh "$(VERSION)"

publish:
	./scripts/publish-gitlab.sh "$(VERSION)"

clean:
	rm -f bin/aissh
	rm -rf dist
