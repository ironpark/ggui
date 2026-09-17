GO ?= go

.PHONY: all build test vet fmt tidy run run-todo run-gallery clean

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

run-todo:
	$(GO) run ./examples/todo

run-gallery:
	$(GO) run ./examples/gallery

clean:
	$(GO) clean
	rm -rf bin
