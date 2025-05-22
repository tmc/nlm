.PHONY: all build test clean beproto

all: build

build:
	go build -o nlm ./cmd/nlm

beproto:
	go build -o beproto ./internal/cmd/beproto

test:
	go test ./...

clean:
	rm -f nlm beproto