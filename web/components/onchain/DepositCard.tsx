'use client';

// DepositCard — Step 2/3 of the landing page: pick a level + deposit USDR
// into GameVault. Two-step flow: approve, then deposit.
//
// After a successful deposit we poll GET /sessions/{addr} on the backend
// until it returns `ready: true`, then redirect to /trade?session=...&address=...
// — that gives the indexer enough time to confirm 5 blocks deep.

import { useEffect, useMemo, useState } from 'react';
import { formatUnits, parseUnits } from 'viem';
import {
  useAccount,
  useReadContract,
  useWaitForTransactionReceipt,
  useWriteContract,
} from 'wagmi';
import { useRouter } from 'next/navigation';
import { gameVaultAbi, usdrAbi } from '@/lib/abi';
import type { Deployment } from '@/lib/deployments';
import { LEVELS, levelIdHash, type Level } from '@/lib/levels';
import { API_BASE } from '@/lib/config';
import { txLink } from '@/lib/chains';

type Props = { deployment: Deployment };

type Phase = 'idle' | 'approving' | 'approve-confirmed' | 'depositing' | 'waiting-indexer' | 'ready' | 'error';

const SESSION_POLL_INTERVAL_MS = 2_000;
const SESSION_POLL_TIMEOUT_MS = 90_000; // 5 blocks * ~250ms on Arbitrum + slack

export default function DepositCard({ deployment }: Props) {
  const router = useRouter();
  const { address, isConnected } = useAccount();
  const [level, setLevel] = useState<Level>(LEVELS[0]);
  const [amount, setAmount] = useState<string>(LEVELS[0].defaultDeposit);
  const [phase, setPhase] = useState<Phase>('idle');
  const [errMsg, setErrMsg] = useState<string | null>(null);
  const [pollStartedAt, setPollStartedAt] = useState<number | null>(null);

  // Convert the user's whole-USDR string into 18-decimal wei. Memoised so we
  // don't do the parse on every render.
  const amountWei = useMemo(() => {
    try {
      return parseUnits(amount || '0', 18);
    } catch {
      return 0n;
    }
  }, [amount]);

  const allowance = useReadContract({
    address: deployment.usdr,
    abi: usdrAbi,
    functionName: 'allowance',
    args: address ? [address, deployment.vault] : undefined,
    query: { enabled: !!address, refetchInterval: 4_000 },
  });
  const usdrBalance = useReadContract({
    address: deployment.usdr,
    abi: usdrAbi,
    functionName: 'balanceOf',
    args: address ? [address] : undefined,
    query: { enabled: !!address, refetchInterval: 8_000 },
  });

  const needsApprove = (allowance.data ?? 0n) < amountWei;

  const approveTx = useWriteContract();
  const approveReceipt = useWaitForTransactionReceipt({ hash: approveTx.data });
  const depositTx = useWriteContract();
  const depositReceipt = useWaitForTransactionReceipt({ hash: depositTx.data });

  // ---- approve flow
  const handleApprove = () => {
    setErrMsg(null);
    setPhase('approving');
    approveTx.writeContract(
      {
        address: deployment.usdr,
        abi: usdrAbi,
        functionName: 'approve',
        args: [deployment.vault, amountWei],
      },
      {
        onError: (err) => {
          setErrMsg(humanize(err));
          setPhase('error');
        },
      },
    );
  };

  useEffect(() => {
    if (approveReceipt.isSuccess && phase === 'approving') {
      setPhase('approve-confirmed');
      allowance.refetch();
    }
  }, [approveReceipt.isSuccess]); // eslint-disable-line react-hooks/exhaustive-deps

  // ---- deposit flow
  const handleDeposit = () => {
    setErrMsg(null);
    setPhase('depositing');
    depositTx.writeContract(
      {
        address: deployment.vault,
        abi: gameVaultAbi,
        functionName: 'deposit',
        args: [levelIdHash(level.id), amountWei],
      },
      {
        onError: (err) => {
          setErrMsg(humanize(err));
          setPhase('error');
        },
      },
    );
  };

  useEffect(() => {
    if (depositReceipt.isSuccess && phase === 'depositing') {
      setPhase('waiting-indexer');
      setPollStartedAt(Date.now());
      usdrBalance.refetch();
      allowance.refetch();
    }
  }, [depositReceipt.isSuccess]); // eslint-disable-line react-hooks/exhaustive-deps

  // ---- post-deposit poll for /sessions/{addr}
  useEffect(() => {
    if (phase !== 'waiting-indexer' || !address) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout>;

    const poll = async () => {
      if (cancelled) return;
      try {
        const r = await fetch(`${API_BASE}/sessions/${address}`);
        if (r.status === 200) {
          const j = await r.json();
          if (!cancelled && j.ready && j.session?.session_id) {
            setPhase('ready');
            const sid: string = j.session.session_id;
            router.push(`/trade?session=${encodeURIComponent(sid)}&address=${address}`);
            return;
          }
        }
        // 202 (chain confirmed, actor not yet caught up) or 404 (not yet
        // confirmed) → keep polling.
      } catch {
        // network blip — keep polling
      }

      // Bail out after timeout.
      if (pollStartedAt && Date.now() - pollStartedAt > SESSION_POLL_TIMEOUT_MS) {
        if (!cancelled) {
          setPhase('error');
          setErrMsg(
            'Backend indexer did not pick up your deposit within 90s. Refresh and try again, or check the backend logs.',
          );
        }
        return;
      }
      timer = setTimeout(poll, SESSION_POLL_INTERVAL_MS);
    };
    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [phase, address, pollStartedAt, router]);

  const balanceUSDR = usdrBalance.data ? Number(formatUnits(usdrBalance.data as bigint, 18)) : 0;
  const insufficient = balanceUSDR < parseFloat(amount || '0');

  return (
    <div className="rounded border border-gray-800 bg-[#111] p-5">
      <h2 className="text-base font-semibold mb-4">Step 2 — Pick a level &amp; deposit</h2>

      <label className="block text-xs text-gray-500 mb-1">Level</label>
      <select
        value={level.id}
        onChange={(e) => setLevel(LEVELS.find((l) => l.id === e.target.value) || LEVELS[0])}
        className="w-full bg-[#0f0f0f] border border-gray-700 rounded px-3 py-2 text-sm mb-3 text-white"
        disabled={phase === 'waiting-indexer' || phase === 'ready'}
      >
        {LEVELS.map((l) => (
          <option key={l.id} value={l.id}>
            {l.title} ({l.difficulty})
          </option>
        ))}
      </select>
      <p className="text-xs text-gray-500 mb-4">{level.description}</p>

      <label className="block text-xs text-gray-500 mb-1">Deposit (USDR)</label>
      <input
        type="number"
        min="100"
        max="50000"
        step="50"
        value={amount}
        onChange={(e) => setAmount(e.target.value)}
        className="w-full bg-[#0f0f0f] border border-gray-700 rounded px-3 py-2 text-sm font-mono text-white mb-1"
        disabled={phase === 'waiting-indexer' || phase === 'ready'}
      />
      <div className="text-xs text-gray-600 mb-4">
        Min 100, max 50,000. You have {balanceUSDR.toLocaleString()} USDR.
      </div>

      {needsApprove ? (
        <button
          type="button"
          disabled={!isConnected || insufficient || approveTx.isPending || approveReceipt.isLoading}
          onClick={handleApprove}
          className="w-full py-2.5 rounded bg-yellow-500 text-black font-semibold text-sm disabled:bg-gray-700 disabled:text-gray-400 hover:bg-yellow-400"
        >
          {approveTx.isPending
            ? 'Sign approve…'
            : approveReceipt.isLoading
              ? 'Approving…'
              : `1/2 · Approve ${amount} USDR`}
        </button>
      ) : (
        <button
          type="button"
          disabled={
            !isConnected ||
            insufficient ||
            depositTx.isPending ||
            depositReceipt.isLoading ||
            phase === 'waiting-indexer' ||
            phase === 'ready'
          }
          onClick={handleDeposit}
          className="w-full py-2.5 rounded bg-[#26a69a] text-black font-semibold text-sm disabled:bg-gray-700 disabled:text-gray-400 hover:bg-[#2db8aa]"
        >
          {depositTx.isPending
            ? 'Sign deposit…'
            : depositReceipt.isLoading
              ? 'Depositing…'
              : phase === 'waiting-indexer'
                ? 'Waiting for backend (5 blocks)…'
                : phase === 'ready'
                  ? 'Redirecting…'
                  : `2/2 · Deposit & Start ${level.title}`}
        </button>
      )}

      {approveTx.data && (
        <a
          href={txLink(approveTx.data)}
          target="_blank"
          rel="noreferrer"
          className="block text-xs text-gray-500 hover:text-white mt-2 truncate font-mono"
        >
          approve tx: {approveTx.data}
        </a>
      )}
      {depositTx.data && (
        <a
          href={txLink(depositTx.data)}
          target="_blank"
          rel="noreferrer"
          className="block text-xs text-gray-500 hover:text-white mt-2 truncate font-mono"
        >
          deposit tx: {depositTx.data}
        </a>
      )}
      {errMsg && <div className="text-xs text-red-400 mt-3">{errMsg}</div>}
    </div>
  );
}

function humanize(err: unknown): string {
  if (!err) return 'Unknown error';
  const e = err as { shortMessage?: string; message?: string };
  return (e.shortMessage || e.message || 'Wallet rejected or RPC error').slice(0, 240);
}
