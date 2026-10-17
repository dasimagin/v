BIN := v

.PHONY: build run lint fmt clean

build:
	go build -o $(BIN) .

run: build
	./$(BIN)

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

clean:
	rm -f $(BIN)
