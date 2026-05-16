.PHONY: all build build-server build-cli build-web test bench replay server cli dev backend frontend clean fmt vet tidy \
	contracts contracts-test contracts-fmt contracts-build contracts-deploy sync-deployments

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

build-web:
	cd web && npm run build

test:
	$(GO) test -race -count=1 -coverprofile=coverage.out $(PKG)

bench:
	$(GO) test -bench=. -benchmem -run=^$$ ./internal/orderbook/...

replay: build
	@mkdir -p $(OUT_DIR)
	$(BIN_DIR)/replay --level=$(or $(LEVEL),D-312-BTC) --speed=$(or $(SPEED),60x) --provider=$(or $(PROVIDER),mock) --out=$(OUT_DIR)

# v0.3: one-command startup — runs backend + frontend in parallel.
# v0.5: pass CHAIN_RPC=https://... to enable on-chain entry mode.
# Press Ctrl-C twice (or kill the background PID) to stop.
dev:
	@echo "Starting backend on :8080 and frontend on :3000"
	@(make backend &) && make frontend

backend: build-server sync-deployments
	@mkdir -p $(OUT_DIR)
	$(BIN_DIR)/server \
		--level=$(or $(LEVEL),D-312-BTC) \
		--seed=$(or $(SEED),42) \
		--port=$(or $(PORT),8080) \
		--balance=$(or $(BALANCE),10000) \
		$(if $(CHAIN_RPC),--chain-rpc=$(CHAIN_RPC))

frontend: sync-deployments
	cd web && npm run dev

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

# ------------------------------------------------------------
# v0.5 — Smart contracts (Foundry)
# ------------------------------------------------------------

contracts: contracts-build contracts-test ## build + test contracts

contracts-build:
	cd contracts && forge build

contracts-fmt:
	cd contracts && forge fmt --check

contracts-test:
	cd contracts && forge test -vv

# Deploy with: `make contracts-deploy CHAIN=arbitrum_sepolia`
# Requires contracts/.env populated (see contracts/.env.example).
contracts-deploy:
	@if [ -z "$(CHAIN)" ]; then echo "set CHAIN=arbitrum_sepolia (or sepolia)"; exit 2; fi
	cd contracts && bash -c 'set -a && . ./.env && set +a && \
		forge script script/Deploy.s.sol:Deploy \
		--rpc-url "$$ARBITRUM_SEPOLIA_RPC_URL" \
		--private-key "$$PRIVATE_KEY" \
		--broadcast --verify'
	@echo "Deploy complete. Sync deployments → frontend with: make sync-deployments"

# Copy the canonical deployments JSON into the FE public/ folder so the
# browser can fetch it at runtime. Run automatically by backend/frontend
# targets so devs always have fresh addresses.
sync-deployments:
	@mkdir -p web/public/deployments
	@cp deployments/arbitrum-sepolia.json web/public/deployments/arbitrum-sepolia.json 2>/dev/null || true
