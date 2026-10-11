.PHONY: all build test vet fmt clean

all: vet test build

build:
	go build -o bin/goreadmenator ./cmd/goreadmenator

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

clean:
	rm -rf bin/
