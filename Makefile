BINARY_NAME := lekture
GO := go

.PHONY: build run clean tidy test

build:
	$(GO) build -o $(BINARY_NAME) .

run: build
	./$(BINARY_NAME) $(ARGS)

clean:
	rm -f $(BINARY_NAME)

tidy:
	$(GO) mod tidy

test:
	$(GO) test ./...
