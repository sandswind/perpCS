'use client';

// SessionEndModal — shown when a session_close WS event arrives or the player
// manually clicks "End Session". Displays final equity and a Withdraw button.
//
// v0.6: Withdraw calls POST /sessions/{addr}/close (if not already done) then
// redirects to /report?address={addr}&session={sid}.
// No on-chain tx needed for the MVP — the backend mock receipt is sufficient.

import { useState, useCallback } from 'react';
import { API_BASE } from '@/lib/config';

export interface SessionCloseEvent {
  session_id: string;
  final_equity: string; // human-readable USDR
  ts: number;
}

interface Props {
  event: SessionCloseEvent | null;
  address: string;
  sessionId: string;
  initialBalance: number;
  onClose: () => void;
}

type WithdrawState = 'idle' | 'loading' | 'done' | 'error';

export default function SessionEndModal({
  event,
  address,
  sessionId,
  initialBalance,
  onClose,
}: Props) {
  const [withdrawState, setWithdrawState] = useState<WithdrawState>('idle');
  const [receipt, setReceipt] = useState<Record<string, unknown> | null>(null);
  const [errorMsg, setErrorMsg] = useState('');

  const finalEquity = event ? parseFloat(event.final_equity) : 0;
  const pnl = finalEquity - initialBalance;
  const pnlPct = initialBalance > 0 ? (pnl / initialBalance) * 100 : 0;
  const survived = finalEquity > initialBalance * 0.01; // > 1% → survived

  const handleWithdraw = useCallback(async () => {
    setWithdrawState('loading');
    setErrorMsg('');
    try {
      // 1. Close + settle the session (idempotent if already closed)
      const addrParam = address || 'player1';
      const closeRes = await fetch(`${API_BASE}/sessions/${addrParam}/close`, {
        method: 'POST',
      });
      if (!closeRes.ok) {
        const body = await closeRes.json().catch(() => ({}));
        throw new Error((body as { error?: string }).error || `HTTP ${closeRes.status}`);
      }
      const rec = await closeRes.json();
      setReceipt(rec as Record<string, unknown>);
      setWithdrawState('done');

      // 2. Redirect to debrief report after short delay
      setTimeout(() => {
        const params = new URLSearchParams();
        if (address) params.set('address', address);
        if (sessionId) params.set('session', sessionId);
        window.location.href = `/report?${params.toString()}`;
      }, 1800);
    } catch (err) {
      setErrorMsg(String(err instanceof Error ? err.message : err));
      setWithdrawState('error');
    }
  }, [address, sessionId]);

  if (!event) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/85 backdrop-blur-sm"
      role="dialog"
      aria-modal="true"
    >
      <div
        className="relative max-w-md w-full mx-4 rounded-2xl border shadow-2xl overflow-hidden"
        style={{
          background: '#111',
          borderColor: survived ? '#26a69a' : '#ef5350',
        }}
      >
        {/* Header stripe */}
        <div
          className="px-6 py-4 flex items-center gap-3"
          style={{ background: survived ? '#26a69a22' : '#ef535022' }}
        >
          <span className="text-4xl">{survived ? '🏆' : '💀'}</span>
          <div>
            <h2
              className="text-xl font-bold"
              style={{ color: survived ? '#26a69a' : '#ef5350' }}
            >
              {survived ? 'Session Complete' : 'Session Over'}
            </h2>
            <p className="text-xs text-gray-400 font-mono mt-0.5">
              {event.session_id
                ? `${event.session_id.slice(0, 10)}…${event.session_id.slice(-6)}`
                : sessionId}
            </p>
          </div>
        </div>

        {/* Stats */}
        <div className="px-6 py-5 space-y-3">
          <StatRow
            label="Initial Balance"
            value={`${initialBalance.toLocaleString(undefined, { maximumFractionDigits: 2 })} USDR`}
            dimmed
          />
          <StatRow
            label="Final Equity"
            value={`${finalEquity.toLocaleString(undefined, { maximumFractionDigits: 4 })} USDR`}
            bold
          />
          <div className="border-t border-gray-800 pt-3">
            <StatRow
              label="Realized PnL"
              value={`${pnl >= 0 ? '+' : ''}${pnl.toLocaleString(undefined, { maximumFractionDigits: 4 })} USDR (${pnl >= 0 ? '+' : ''}${pnlPct.toFixed(2)}%)`}
              positive={pnl >= 0}
              negative={pnl < 0}
              bold
            />
          </div>
        </div>

        {/* Actions */}
        <div className="px-6 pb-6 space-y-3">
          {withdrawState === 'done' && receipt && (
            <div className="rounded-lg border border-green-800 bg-green-900/20 p-3 text-xs font-mono text-green-400 break-all">
              <div className="font-semibold mb-1">✓ Settlement complete — redirecting to debrief…</div>
              <div className="text-green-600">
                Final Equity:{' '}
                {String((receipt as { final_equity?: string }).final_equity ?? finalEquity)} USDR
              </div>
            </div>
          )}

          {withdrawState === 'error' && (
            <div className="rounded-lg border border-red-800 bg-red-900/20 p-3 text-xs text-red-400">
              {errorMsg}
            </div>
          )}

          {withdrawState !== 'done' && (
            <button
              onClick={handleWithdraw}
              disabled={withdrawState === 'loading'}
              className="w-full py-3 rounded-xl font-semibold text-sm transition-all disabled:opacity-50"
              style={{
                background: survived ? '#26a69a' : '#ef5350',
                color: '#000',
              }}
            >
              {withdrawState === 'loading' ? (
                <span className="inline-flex items-center gap-2">
                  <span className="animate-spin inline-block w-3 h-3 border-2 border-black/30 border-t-black rounded-full" />
                  Settling…
                </span>
              ) : (
                '↩ Withdraw & View Debrief'
              )}
            </button>
          )}

          <button
            onClick={onClose}
            className="w-full py-2 rounded-xl text-xs text-gray-500 hover:text-gray-300 transition-colors border border-gray-800 hover:border-gray-600"
          >
            Continue Trading
          </button>
        </div>
      </div>
    </div>
  );
}

// ---- helpers ----

function StatRow({
  label,
  value,
  bold,
  dimmed,
  positive,
  negative,
}: {
  label: string;
  value: string;
  bold?: boolean;
  dimmed?: boolean;
  positive?: boolean;
  negative?: boolean;
}) {
  const valColor = positive
    ? '#26a69a'
    : negative
      ? '#ef5350'
      : dimmed
        ? '#555'
        : '#d1d4dc';

  return (
    <div className="flex items-center justify-between">
      <span className="text-xs text-gray-500">{label}</span>
      <span
        className={`text-sm font-mono ${bold ? 'font-semibold' : 'font-normal'}`}
        style={{ color: valColor }}
      >
        {value}
      </span>
    </div>
  );
}
