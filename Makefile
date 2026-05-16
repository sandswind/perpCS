.PHONY: all build build-server build-cli test bench replay server cli clean fmt vet tidy

GO       ?= go
PKG      := ./...
BIN_DIR  := bin
OUT_DIR  := out

all: fmt vet test build

build: build-server build-cli
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/replay ./cmd/replay

build-server:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/server ./cmd/server

build-cli:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/cli ./cmd/cli

test:
	$(GO) test -race -count=1 -coverprofile=coverage.out $(PKG)

bench:
	$(GO) test -bench=. -benchmem -run=^$$ ./internal/orderbook/...

replay: build
	@mkdir -p $(OUT_DIR)
	$(BIN_DIR)/replay --level=$(or $(LEVEL),D-312-BTC) --speed=$(or $(SPEED),60x) --provider=$(or $(PROVIDER),mock) --out=$(OUT_DIR)

server: build-server
	$(BIN_DIR)/server --level=$(or $(LEVEL),D-312-BTC) --seed=$(or $(SEED),42) --port=$(or $(PORT),8080)

cli: build-cli
	$(BIN_DIR)/cli --server=$(or $(SERVER),http://localhost:8080)

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
