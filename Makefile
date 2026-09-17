BINARY_NAME := lekture
GO := go
VHS := vhs

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build run clean tidy test race fmt vet check check-vhs gif screenshots assets

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY_NAME) .

run: build
	./$(BINARY_NAME) $(ARGS)

clean:
	rm -f $(BINARY_NAME)

tidy:
	$(GO) mod tidy

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# check runs everything CI runs.
check: fmt vet race build

# vhs 0.12.0 has a regression: it runs a tape, prints "Creating ...", exits 0
# and writes no file. Fail loudly rather than appearing to succeed.
#   https://github.com/charmbracelet/vhs/issues/787
# Pin a working release with:
#   go install github.com/charmbracelet/vhs@v0.11.0
check-vhs:
	@command -v $(VHS) >/dev/null || { \
		echo "vhs not found — install it with: go install github.com/charmbracelet/vhs@v0.11.0"; \
		exit 1; \
	}
	@$(VHS) --version | grep -q '0\.12\.0' && { \
		echo "vhs 0.12.0 exits 0 without writing any output (charmbracelet/vhs#787)."; \
		echo "install a working release: go install github.com/charmbracelet/vhs@v0.11.0"; \
		exit 1; \
	} || true

gif: check-vhs
	$(VHS) vhs/cassette.tape

screenshots: check-vhs
	$(VHS) vhs/screenshots.tape

assets: gif screenshots
