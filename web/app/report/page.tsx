'use client';

// Debrief Report Page — /report?address=0x...&session=0x...
//
// Shows:
//  - Final equity + realized PnL + max drawdown
//  - PnL curve (SVG chart)
//  - Trade count + liquidation status
//  - WithdrawReceipt details (session_id, signer, signature)
//  - Links back to lobby and to start a new session

import { Suspense } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import dynamic from 'next/dynamic';
import { useReport } from '@/hooks/useReport';

const PnLChart = dynamic(() => import('@/components/PnLChart'), { ssr: false });

function ReportView() {
  const sp = useSearchParams();
  const address = sp.get('address') || '';
  const sessionId = sp.get('session') || '';

  const { report, loading, error } = useReport(address, sessionId);

  const displayAddr = address
    ? `${address.slice(0, 8)}…${address.slice(-6)}`
    : 'demo';

  return (
    <div
      className="min-h-screen w-screen overflow-y-auto font-sans"
      style={{ backgroundColor: '#0f0f0f', color: '#d1d4dc' }}
    >
      {/* Header */}
      <header
        className="sticky top-0 z-10 flex items-center justify-between px-6 py-3 border-b border-gray-800"
        style={{ background: '#111' }}
      >
        <div className="flex items-center gap-4">
          <Link
            href="/"
            className="text-gray-500 hover:text-white text-sm transition-colors"
          >
            ← Lobby
          </Link>
          <span className="text-gray-700">|</span>
          <h1 className="text-sm font-semibold text-white">Session Debrief</h1>
        </div>
        <span className="text-xs text-gray-500 font-mono">{displayAddr}</span>
      </header>

      <main className="max-w-4xl mx-auto px-6 py-8 space-y-6">
        {loading && (
          <div className="flex items-center justify-center py-24 text-gray-500 text-sm">
            <span className="animate-spin inline-block w-5 h-5 border-2 border-gray-700 border-t-gray-400 rounded-full mr-3" />
            Loading debrief…
          </div>
        )}

        {error && (
          <div className="rounded-xl border border-red-800 bg-red-900/15 p-5">
            <div className="text-red-400 font-semibold mb-1">Could not load report</div>
            <p className="text-sm text-red-300/70">{error}</p>
            <p className="text-xs text-gray-600 mt-3">
              The session may still be running.{' '}
              <Link href={`/trade?address=${address}&session=${sessionId}`} className="underline hover:text-gray-400">
                Return to trading
              </Link>
            </p>
          </div>
        )}

        {!loading && !error && report && (
          <>
            {/* Status banner */}
            <div
              className="rounded-2xl p-5 flex items-center gap-4 border"
              style={{
                background: report.was_liquidated ? '#ef535010' : '#26a69a10',
                borderColor: report.was_liquidated ? '#ef535040' : '#26a69a40',
              }}
            >
              <span className="text-5xl">
                {report.was_liquidated ? '💀' : '🏆'}
              </span>
              <div>
                <h2
                  className="text-2xl font-bold"
                  style={{ color: report.was_liquidated ? '#ef5350' : '#26a69a' }}
                >
                  {report.was_liquidated ? 'Liquidated' : 'Survived'}
                </h2>
                <p className="text-sm text-gray-400 mt-0.5">
                  {report.was_liquidated
                    ? 'Your position was force-closed by the liquidation engine.'
                    : 'You made it through the crisis with some equity intact.'}
                </p>
              </div>
            </div>

            {/* Key metrics grid */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <MetricCard
                label="Final Equity"
                value={`${parseFloat(report.receipt?.final_equity ?? '0').toLocaleString(undefined, { maximumFractionDigits: 2 })} USDR`}
                accent
              />
              <MetricCard
                label="Realized PnL"
                value={`${parseFloat(report.realized_pnl) >= 0 ? '+' : ''}${parseFloat(report.realized_pnl).toLocaleString(undefined, { maximumFractionDigits: 2 })} USDR`}
                positive={parseFloat(report.realized_pnl) >= 0}
                negative={parseFloat(report.realized_pnl) < 0}
              />
              <MetricCard
                label="Max Drawdown"
                value={`${(parseFloat(report.max_drawdown) * 100).toFixed(2)}%`}
                negative={parseFloat(report.max_drawdown) > 0.2}
              />
              <MetricCard
                label="Total Trades"
                value={String(report.total_trades)}
              />
            </div>

            {/* PnL Curve */}
            <div
              className="rounded-xl border border-gray-800 overflow-hidden"
              style={{ background: '#111' }}
            >
              <div className="px-4 py-3 border-b border-gray-800 flex items-center justify-between">
                <span className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                  PnL Curve
                </span>
                <span className="text-xs text-gray-600 font-mono">USDR vs chaos clock</span>
              </div>
              <div className="p-4">
                {report.pnl_curve.length >= 2 ? (
                  <PnLChart data={report.pnl_curve} height={220} />
                ) : (
                  <div className="flex items-center justify-center h-24 text-xs text-gray-600">
                    Not enough data points to render curve
                  </div>
                )}
              </div>
            </div>

            {/* Receipt card */}
            {report.receipt && (
              <div
                className="rounded-xl border border-gray-800 p-5 space-y-3"
                style={{ background: '#111' }}
              >
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-gray-400 uppercase tracking-wide">
                    Settlement Receipt
                  </span>
                  {report.receipt.signature ? (
                    <span className="text-xs px-2 py-0.5 rounded-full bg-green-900/30 text-green-400 border border-green-800">
                      Signed
                    </span>
                  ) : (
                    <span className="text-xs px-2 py-0.5 rounded-full bg-gray-800 text-gray-500 border border-gray-700">
                      Unsigned (MVP)
                    </span>
                  )}
                </div>

                <ReceiptRow label="Session ID" value={report.receipt.session_id} mono truncate />
                <ReceiptRow label="Player" value={report.receipt.player || address || '—'} mono truncate />
                <ReceiptRow label="Final Equity" value={`${report.receipt.final_equity} USDR`} mono />
                <ReceiptRow label="Chain ID" value={String(report.receipt.chain_id)} mono />
                {report.receipt.signer && (
                  <ReceiptRow label="Signer" value={report.receipt.signer} mono truncate />
                )}
                {report.receipt.signature && (
                  <ReceiptRow
                    label="Signature"
                    value={`${report.receipt.signature.slice(0, 18)}…`}
                    mono
                    dimmed
                  />
                )}
              </div>
            )}

            {/* Actions */}
            <div className="flex flex-col sm:flex-row gap-3 pt-2">
              <Link
                href="/"
                className="flex-1 py-3 rounded-xl text-center text-sm font-semibold border border-gray-700 text-gray-300 hover:text-white hover:border-gray-500 transition-colors"
              >
                ← Back to Lobby
              </Link>
              <Link
                href={`/trade?address=${address}&session=${sessionId}`}
                className="flex-1 py-3 rounded-xl text-center text-sm font-semibold border border-gray-700 text-gray-300 hover:text-white hover:border-gray-500 transition-colors"
              >
                Play Again
              </Link>
            </div>
          </>
        )}
      </main>
    </div>
  );
}

export default function ReportPage() {
  return (
    <Suspense fallback={<div className="p-8 text-gray-500 text-sm">Loading…</div>}>
      <ReportView />
    </Suspense>
  );
}

// ---- helpers ----

function MetricCard({
  label,
  value,
  accent,
  positive,
  negative,
}: {
  label: string;
  value: string;
  accent?: boolean;
  positive?: boolean;
  negative?: boolean;
}) {
  const valColor = positive
    ? '#26a69a'
    : negative
      ? '#ef5350'
      : accent
        ? '#d1d4dc'
        : '#9ca3af';

  return (
    <div
      className="rounded-xl border border-gray-800 p-4 flex flex-col gap-1"
      style={{ background: '#111' }}
    >
      <span className="text-xs text-gray-500 uppercase tracking-wide">{label}</span>
      <span className="text-base font-bold font-mono" style={{ color: valColor }}>
        {value}
      </span>
    </div>
  );
}

function ReceiptRow({
  label,
  value,
  mono,
  truncate,
  dimmed,
}: {
  label: string;
  value: string;
  mono?: boolean;
  truncate?: boolean;
  dimmed?: boolean;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <span className="text-xs text-gray-500 flex-shrink-0 pt-0.5">{label}</span>
      <span
        className={`text-xs text-right ${mono ? 'font-mono' : ''} ${dimmed ? 'text-gray-600' : 'text-gray-300'} ${truncate ? 'truncate max-w-[280px]' : ''}`}
        title={value}
      >
        {value}
      </span>
    </div>
  );
}
