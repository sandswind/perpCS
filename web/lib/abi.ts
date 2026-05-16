// Minimal ABI snippets for the 3 v0.5 contracts.
//
// We only declare the entries the FE actually calls; full ABIs live in
// contracts/out/<Contract>.sol/<Contract>.json after `forge build`.

export const usdrAbi = [
  {
    type: 'function',
    name: 'balanceOf',
    stateMutability: 'view',
    inputs: [{ name: 'account', type: 'address' }],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'function',
    name: 'allowance',
    stateMutability: 'view',
    inputs: [
      { name: 'owner', type: 'address' },
      { name: 'spender', type: 'address' },
    ],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'function',
    name: 'approve',
    stateMutability: 'nonpayable',
    inputs: [
      { name: 'spender', type: 'address' },
      { name: 'value', type: 'uint256' },
    ],
    outputs: [{ name: '', type: 'bool' }],
  },
  {
    type: 'function',
    name: 'decimals',
    stateMutability: 'view',
    inputs: [],
    outputs: [{ name: '', type: 'uint8' }],
  },
  {
    type: 'function',
    name: 'symbol',
    stateMutability: 'view',
    inputs: [],
    outputs: [{ name: '', type: 'string' }],
  },
] as const;

export const faucetAbi = [
  {
    type: 'function',
    name: 'claim',
    stateMutability: 'nonpayable',
    inputs: [],
    outputs: [],
  },
  {
    type: 'function',
    name: 'hasClaimed',
    stateMutability: 'view',
    inputs: [{ name: 'account', type: 'address' }],
    outputs: [{ name: '', type: 'bool' }],
  },
  {
    type: 'function',
    name: 'CLAIM_AMOUNT',
    stateMutability: 'view',
    inputs: [],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'function',
    name: 'remainingBalance',
    stateMutability: 'view',
    inputs: [],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'event',
    name: 'Claimed',
    inputs: [
      { name: 'account', type: 'address', indexed: true },
      { name: 'amount', type: 'uint256', indexed: false },
      { name: 'totalClaimed', type: 'uint256', indexed: false },
    ],
  },
  // Custom errors so wallets can decode revert reasons.
  { type: 'error', name: 'AlreadyClaimed', inputs: [] },
  {
    type: 'error',
    name: 'InsufficientFaucetBalance',
    inputs: [
      { name: 'have', type: 'uint256' },
      { name: 'need', type: 'uint256' },
    ],
  },
] as const;

export const gameVaultAbi = [
  {
    type: 'function',
    name: 'deposit',
    stateMutability: 'nonpayable',
    inputs: [
      { name: 'levelId', type: 'bytes32' },
      { name: 'amount', type: 'uint256' },
    ],
    outputs: [{ name: 'sessionId', type: 'bytes32' }],
  },
  {
    type: 'function',
    name: 'depositNonce',
    stateMutability: 'view',
    inputs: [{ name: 'player', type: 'address' }],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'function',
    name: 'MIN_DEPOSIT',
    stateMutability: 'view',
    inputs: [],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'function',
    name: 'MAX_DEPOSIT',
    stateMutability: 'view',
    inputs: [],
    outputs: [{ name: '', type: 'uint256' }],
  },
  {
    type: 'event',
    name: 'SessionStarted',
    inputs: [
      { name: 'player', type: 'address', indexed: true },
      { name: 'levelId', type: 'bytes32', indexed: true },
      { name: 'sessionId', type: 'bytes32', indexed: true },
      { name: 'amount', type: 'uint256', indexed: false },
      { name: 'nonce', type: 'uint256', indexed: false },
      { name: 'blockNumber', type: 'uint256', indexed: false },
    ],
  },
  {
    type: 'error',
    name: 'AmountBelowMin',
    inputs: [
      { name: 'amount', type: 'uint256' },
      { name: 'min', type: 'uint256' },
    ],
  },
  {
    type: 'error',
    name: 'AmountAboveMax',
    inputs: [
      { name: 'amount', type: 'uint256' },
      { name: 'max', type: 'uint256' },
    ],
  },
] as const;
