.PHONY: build run test clean fmt lint

BINARY := quizhub
MAIN   := ./cmd/server

build:
	go build -o $(BINARY) $(MAIN)

run: build
	./$(BINARY)

test:
	go test ./... -v -count=1

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY) coverage.out *.db
