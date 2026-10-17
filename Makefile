BIN := v

.PHONY: build run test lint fmt clean

build:
	go build -o $(BIN) .

run: build
	./$(BIN)

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

clean:
	rm -f $(BIN)
