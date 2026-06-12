BINARY_NAME := lekture
GO := go
VHS := vhs

.PHONY: build run clean tidy test gif screenshots assets

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

gif:
	$(VHS) vhs/cassette.tape

screenshots:
	$(VHS) vhs/screenshots.tape

assets: gif screenshots
