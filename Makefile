BINARY := web3load

.PHONY: build test lint fmt run-example clean

build:
	go build -o bin/$(BINARY) ./cmd/web3load

test:
	go test ./... -race

lint:
	golangci-lint run

fmt:
	gofmt -l -w .

run-example: build
	./bin/$(BINARY) validate examples/native_transfer.yaml

clean:
	rm -rf bin/
