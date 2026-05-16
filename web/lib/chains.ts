// Chain definitions for the v0.5 on-chain entry path.
//
// We use viem's built-in arbitrumSepolia. Add additional chains here when we
// support them; the Wagmi config in providers.tsx must match.

import { arbitrumSepolia } from 'wagmi/chains';

export const SUPPORTED_CHAINS = [arbitrumSepolia] as const;
export const DEFAULT_CHAIN = arbitrumSepolia;
export const DEFAULT_CHAIN_ID = arbitrumSepolia.id; // 421614

// Block explorer link helpers (used by the FE to deep-link into Arbiscan).
export function txLink(hash: `0x${string}`, chainId: number = DEFAULT_CHAIN_ID): string {
  if (chainId === arbitrumSepolia.id) {
    return `https://sepolia.arbiscan.io/tx/${hash}`;
  }
  return `#unknown-chain-${chainId}`;
}

export function addressLink(addr: `0x${string}`, chainId: number = DEFAULT_CHAIN_ID): string {
  if (chainId === arbitrumSepolia.id) {
    return `https://sepolia.arbiscan.io/address/${addr}`;
  }
  return `#unknown-chain-${chainId}`;
}
