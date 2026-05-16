# Deployments

This directory holds `<chain-name>.json` files emitted by
`contracts/script/Deploy.s.sol`. Each file is the source of truth for the
frontend (`web/lib/deployments.ts`) and the Go indexer (`internal/chain`).

## Schema

```json
{
  "chainId": 421614,
  "blockNumber": 12345678,
  "usdr": "0x...",
  "faucet": "0x...",
  "vault": "0x...",
  "vaultFaucet": "0x...",
  "vaultInsurance": "0x...",
  "vaultTreasury": "0x...",
  "vaultTeam": "0x...",
  "vaultLiquidity": "0x..."
}
```

`blockNumber` is the deploy-time block; the indexer uses it as the lower
bound for the historical scan so it doesn't re-fetch uninteresting blocks.

## Re-deploying

Files in this directory are committed. After a re-deploy, commit the
updated JSON in the same PR as any contract changes.

The Solidity script writes the file automatically when run with `--broadcast`.
