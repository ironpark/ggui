GO ?= go

.PHONY: all build test vet fmt tidy run clean

all: fmt vet test

build:
	$(GO) build ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

run:
	$(GO) run ./examples/counter

clean:
	$(GO) clean
	rm -rf bin
