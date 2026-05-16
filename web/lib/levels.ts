// Level catalogue for the FE. Each level corresponds to a (server-side)
// chaos config. The on-chain levelId is keccak256(name).

import { keccak256, stringToBytes } from 'viem';

export type Level = {
  id: string; // e.g. "D-312-BTC"
  symbol: string; // trading pair shown in UI
  title: string;
  difficulty: 'L1' | 'L2' | 'L3';
  description: string;
  defaultDeposit: string; // whole USDR
};

export const LEVELS: Level[] = [
  {
    id: 'D-312-BTC',
    symbol: 'BTC-MED',
    title: 'BTC March 2020',
    difficulty: 'L2',
    description:
      'Replay of the March 12 2020 crash. 5% wicks, depth 20%, 3s oracle lag.',
    defaultDeposit: '500',
  },
];

// keccak256 hash for the GameVault.deposit() bytes32 levelId argument.
export function levelIdHash(id: string): `0x${string}` {
  return keccak256(stringToBytes(id));
}
