// Loads deployed-contract addresses from the JSON file written by
// contracts/script/Deploy.s.sol.
//
// The build pipeline copies deployments/<chain>.json into
// web/public/deployments/<chain>.json (see Makefile target `web-deploys`),
// so the FE can fetch it at runtime — no env-var sprawl, single source of
// truth.

import { DEFAULT_CHAIN, DEFAULT_CHAIN_ID } from './chains';

export type Deployment = {
  chainId: number;
  blockNumber: number;
  usdr: `0x${string}`;
  faucet: `0x${string}`;
  vault: `0x${string}`;
  vaultFaucet?: `0x${string}`;
  vaultInsurance?: `0x${string}`;
  vaultTreasury?: `0x${string}`;
  vaultTeam?: `0x${string}`;
  vaultLiquidity?: `0x${string}`;
};

const FILE_BY_CHAIN: Record<number, string> = {
  [DEFAULT_CHAIN.id]: 'arbitrum-sepolia',
};

export async function loadDeployment(chainId: number = DEFAULT_CHAIN_ID): Promise<Deployment> {
  const slug = FILE_BY_CHAIN[chainId];
  if (!slug) throw new Error(`No deployment file configured for chain ${chainId}`);
  const res = await fetch(`/deployments/${slug}.json`, { cache: 'no-store' });
  if (!res.ok) {
    throw new Error(`Failed to load /deployments/${slug}.json: ${res.status}`);
  }
  const dep = (await res.json()) as Deployment;
  if (
    !dep.usdr ||
    !dep.faucet ||
    !dep.vault ||
    isZeroAddress(dep.usdr) ||
    isZeroAddress(dep.faucet) ||
    isZeroAddress(dep.vault)
  ) {
    throw new Error(
      'Deployment file has zero/missing addresses — run `forge script Deploy.s.sol --broadcast` and commit the updated JSON.',
    );
  }
  return dep;
}

function isZeroAddress(a: string): boolean {
  return /^0x0+$/i.test(a);
}
