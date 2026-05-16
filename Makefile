.PHONY: all build test bench replay clean fmt vet tidy

GO       ?= go
PKG      := ./...
BIN_DIR  := bin
OUT_DIR  := out

all: fmt vet test build

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/replay ./cmd/replay

test:
	$(GO) test -race -count=1 -coverprofile=coverage.out $(PKG)

bench:
	$(GO) test -bench=. -benchmem -run=^$$ ./internal/orderbook/...

replay: build
	@mkdir -p $(OUT_DIR)
	$(BIN_DIR)/replay --level=$(or $(LEVEL),D-312-BTC) --speed=$(or $(SPEED),60x) --provider=$(or $(PROVIDER),mock) --out=$(OUT_DIR)

fmt:
	$(GO) fmt $(PKG)

vet:
	$(GO) vet $(PKG)

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR) $(OUT_DIR) coverage.out

cover-html: test
	$(GO) tool cover -html=coverage.out
