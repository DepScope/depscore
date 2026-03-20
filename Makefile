.PHONY: build test lint clean

build:
	go build -o bin/depscope ./cmd/depscope

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

clean:
	rm -rf bin/
