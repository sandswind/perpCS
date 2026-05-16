'use client';

// FaucetCard — the "Claim 10,000 USDR" panel on the landing page.
//
// State machine (rough):
//   idle           → user has not clicked Claim
//   pending-sign   → wallet is showing the confirm dialog
//   pending-tx     → tx is broadcast, waiting for receipt
//   success        → balance has gone up, button disabled with "Claimed"
//   already        → on mount we found hasClaimed=true; show greyed out
//   error          → revert or RPC error; show message + retry

import { useEffect, useState } from 'react';
import { formatUnits } from 'viem';
import { useAccount, useReadContract, useWaitForTransactionReceipt, useWriteContract } from 'wagmi';
import { faucetAbi, usdrAbi } from '@/lib/abi';
import type { Deployment } from '@/lib/deployments';
import { txLink } from '@/lib/chains';

type Props = { deployment: Deployment };

export default function FaucetCard({ deployment }: Props) {
  const { address, isConnected } = useAccount();
  const [errMsg, setErrMsg] = useState<string | null>(null);

  // Read-only views — refetched after a successful claim.
  const usdrBalance = useReadContract({
    address: deployment.usdr,
    abi: usdrAbi,
    functionName: 'balanceOf',
    args: address ? [address] : undefined,
    query: { enabled: !!address, refetchInterval: 8_000 },
  });

  const claimed = useReadContract({
    address: deployment.faucet,
    abi: faucetAbi,
    functionName: 'hasClaimed',
    args: address ? [address] : undefined,
    query: { enabled: !!address },
  });

  const remaining = useReadContract({
    address: deployment.faucet,
    abi: faucetAbi,
    functionName: 'remainingBalance',
    query: { refetchInterval: 30_000 },
  });

  const { writeContract, data: txHash, isPending: pendingSig, reset } = useWriteContract();
  const { isLoading: pendingTx, isSuccess: txConfirmed } = useWaitForTransactionReceipt({
    hash: txHash,
  });

  // After a successful claim, refresh the wallet's USDR balance + claimed flag.
  useEffect(() => {
    if (txConfirmed) {
      usdrBalance.refetch();
      claimed.refetch();
      remaining.refetch();
    }
  }, [txConfirmed]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleClaim = () => {
    setErrMsg(null);
    reset();
    writeContract(
      {
        address: deployment.faucet,
        abi: faucetAbi,
        functionName: 'claim',
        args: [],
      },
      {
        onError: (err) => {
          setErrMsg(humanizeWalletError(err));
        },
      },
    );
  };

  const balanceUSDR = usdrBalance.data ? formatUnits(usdrBalance.data as bigint, 18) : '—';
  const remainingUSDR = remaining.data
    ? Number(formatUnits(remaining.data as bigint, 18)).toLocaleString(undefined, {
        maximumFractionDigits: 0,
      })
    : '…';

  const alreadyClaimed = claimed.data === true;
  const busy = pendingSig || pendingTx;

  return (
    <div className="rounded border border-gray-800 bg-[#111] p-5">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-base font-semibold">Step 1 — Claim test USDR</h2>
        <span className="text-xs text-gray-500 font-mono">
          faucet pool: {remainingUSDR} USDR
        </span>
      </div>

      <div className="grid grid-cols-2 gap-2 text-xs font-mono mb-4">
        <div className="text-gray-500">Your USDR balance</div>
        <div className="text-right text-white">{Number(balanceUSDR).toLocaleString()}</div>
        <div className="text-gray-500">Faucet status</div>
        <div className="text-right">
          {!isConnected ? (
            <span className="text-gray-500">connect wallet</span>
          ) : alreadyClaimed ? (
            <span className="text-[#26a69a]">already claimed</span>
          ) : (
            <span className="text-yellow-400">unclaimed</span>
          )}
        </div>
      </div>

      <button
        type="button"
        disabled={!isConnected || alreadyClaimed || busy}
        onClick={handleClaim}
        className="w-full py-2.5 rounded bg-[#26a69a] text-black font-semibold text-sm disabled:bg-gray-700 disabled:text-gray-400 hover:bg-[#2db8aa]"
      >
        {!isConnected
          ? 'Connect wallet to continue'
          : alreadyClaimed
            ? 'Already claimed (testnet limit: 1 per wallet)'
            : pendingSig
              ? 'Sign in your wallet…'
              : pendingTx
                ? 'Waiting for confirmation…'
                : 'Claim 10,000 USDR'}
      </button>

      {txHash && (
        <a
          href={txLink(txHash)}
          target="_blank"
          rel="noreferrer"
          className="block text-xs text-gray-400 hover:text-white mt-2 truncate font-mono"
        >
          tx: {txHash}
        </a>
      )}

      {errMsg && <div className="text-xs text-red-400 mt-2">{errMsg}</div>}
    </div>
  );
}

// humanizeWalletError extracts the most useful slice of error info from a
// viem/wagmi error chain.
function humanizeWalletError(err: unknown): string {
  if (!err) return 'Unknown error';
  const e = err as { shortMessage?: string; message?: string; cause?: { message?: string } };
  return (
    e.shortMessage ||
    e.cause?.message ||
    e.message ||
    'Wallet rejected or RPC error'
  ).slice(0, 240);
}
