# perpCS — Perp Crisis Sandbox

A perpetual-futures trading simulator that replays historical disasters
(March 2020, Luna, FTX) with deterministic, server-side chaos: depth shrink,
oracle lag, wick injection, funding boost, liquidation cascades.

## Layout

```
cmd/
  cli/      v0.2 CLI trader
  demo/     deterministic replay demo
  replay/   batch replay → JSONL
  server/   v0.3+ HTTP+WS API + (v0.5) chain indexer
internal/
  account/      isolated-margin AccountStateMachine
  actor/        single-goroutine MarketActor (owns OrderBook + accounts)
  chaos/        seeded chaos engine (no time.Now in hot path)
  fanout/       WebSocket fanout for market + account streams
  orderbook/    matching engine
  provider/     historical data: Binance / mock / synthetic depth
  server/       HTTP API
  types/        domain primitives (Price/Qty in 1e8 fixed-point)
  chain/        v0.5 — JSON-RPC client + Indexer (poll-based, 5-block confirm)
  session/      v0.5 — Indexer → vAccount bridge
contracts/      v0.5 — Foundry workspace (USDR, Faucet, GameVault)
deployments/    v0.5 — per-chain deployment JSON (committed)
web/            v0.3 — Next.js 14 + Tailwind frontend
                v0.5 — Wagmi v2 + RainbowKit + viem + SIWE
```

## Quick start (v0.4 demo, no wallet)

```sh
make dev              # backend on :8080, frontend on :3000
open http://localhost:3000/trade
```

## v0.5 on-chain entry (Arbitrum Sepolia)

The full path is "MetaMask → Faucet → Deposit → Trade".

### 1. Deploy contracts

```sh
cp contracts/.env.example contracts/.env
$EDITOR contracts/.env             # set PRIVATE_KEY + RPC URL

make contracts-deploy CHAIN=arbitrum_sepolia
```

The script writes `deployments/arbitrum-sepolia.json` with the deployed
addresses + the deploy block. Commit it.

### 2. Run the backend with the indexer enabled

```sh
make backend CHAIN_RPC=https://sepolia-rollup.arbitrum.io/rpc
```

The Go indexer polls `eth_blockNumber` + `eth_getLogs` every 4s, scans for
`SessionStarted` logs, waits 5 blocks for confirmation, and forwards each
event to the actor's `SessionQueue` mailbox. The actor creates a fresh
vAccount on its goroutine.

State persists in `out/indexer-state.json` (last processed block) and
`out/sessions.jsonl` (audit log of every confirmed deposit).

### 3. Run the frontend

```sh
cp web/.env.example web/.env.local
$EDITOR web/.env.local             # set NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID

cd web && npm install && npm run dev
```

Visit `http://localhost:3000`:
1. **Connect Wallet** — MetaMask, Rainbow, or any WC wallet.
2. **Switch to Arbitrum Sepolia** if prompted.
3. **Claim 10,000 USDR** — one-time per wallet.
4. **Pick a level**, enter deposit amount (100–50,000 USDR).
5. **Approve + Deposit** — two transactions.
6. The FE polls `GET /sessions/{addr}` until the indexer confirms; you're
   redirected to `/trade?session=...&address=...` automatically (~7s on
   Arbitrum Sepolia).

## Architecture: how the on-chain entry hooks in

```
 ┌─ MetaMask ─┐    ┌─────────────────┐    ┌────────────────┐
 │  approve   │───▶│ USDR.approve()  │    │   GameVault    │
 │  deposit   │───▶│ Vault.deposit() │───▶│ SessionStarted │
 └────────────┘    └─────────────────┘    └────────┬───────┘
                                                   │ event
                                          5 blocks │ confirm
                                                   ▼
                                          ┌────────────────┐
                                          │ chain.Indexer  │  poll, dedupe
                                          └────────┬───────┘
                                                   │ HandleSessionStarted
                                                   ▼
                                          ┌────────────────┐
                                          │  session.Svc   │  audit JSONL
                                          └────────┬───────┘
                                                   │ OpenSessionRequest
                                                   ▼
                                          ┌────────────────┐
                                          │  actor.Actor   │  account.New(...)
                                          └────────┬───────┘
                                                   │ /sessions/{addr} 200 ready
                                                   ▼
                                          ┌────────────────┐
                                          │  FE redirect   │
                                          │ /trade?session │
                                          └────────────────┘
```

## Testing

```sh
make test                # Go (race detector enabled)
make contracts-test      # Foundry (forge test)
cd web && npm run lint   # ESLint
cd web && npm run build  # Next.js production build
```

## Notes

- **Single-writer invariant**: the actor is the only goroutine that mutates
  the `accounts` map and the `OrderBook`. All writes route through channels.
- **Determinism**: the chaos engine derives every random value from
  `(seed, tickIdx)`; same inputs → byte-identical event log.
- **No `go-ethereum` dependency**: the indexer hand-rolls JSON-RPC + the
  one event decoder it needs. Keeps the binary small and the dep tree
  conservative.
