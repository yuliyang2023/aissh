.PHONY: build test clean

GO ?= go

build:
	$(GO) build -o bin/aissh ./cmd/aissh

test:
	$(GO) test ./...

clean:
	rm -f bin/aissh
