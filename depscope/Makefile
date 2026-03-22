.PHONY: build build-lambda test lint clean

build:
	go build -o bin/depscope ./cmd/depscope

build-lambda:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap ./cmd/lambda
	zip lambda.zip bootstrap

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

clean:
	rm -rf bin/ bootstrap lambda.zip
