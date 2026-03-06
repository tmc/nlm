.PHONY: all build test clean install

all: build

build:
	go build -o nlm ./cmd/nlm

install:
	go install ./cmd/nlm

test:
	go test ./...

clean:
	rm -f nlm

generate:
	cd proto && go tool buf generate