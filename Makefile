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

# The graphical UI is embedded from ui/dist, which is committed: rebuild it
# after changing ui/ (needs node).
.PHONY: ui

ui:
	cd ui && npm install && npm run build
