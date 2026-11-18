.PHONY: build test lint clean install fmt

build:
	go build -o bin/senvu ./cmd/senvu

test:
	go test -race ./...

lint:
	go vet ./cmd/... ./internal/... ./pkg/... ./tests/...

clean:
	rm -rf bin/

install:
	go install ./cmd/senvu

fmt:
	gofmt -w cmd internal pkg tests
