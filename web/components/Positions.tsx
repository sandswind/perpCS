'use client';

import type { Position } from '@/hooks/useAccountWS';

interface PositionsProps {
  position: Position | null;
  // Optional mark price (mid of best bid/ask). When present we compute a
  // proper margin ratio to drive the danger styling.
  markPrice?: number;
  // Optional balance — used for margin ratio numerator.
  balance?: number;
}

const MMR = 0.05; // server-side maintenance margin ratio
const WARN = MMR * 1.5; // 7.5% — start flashing red

// computeMarginRatio mirrors the Go-side per-position formula:
//   ratio = (balance + uPnL) / positionNotional
// with positionNotional = size × markPrice. We use account-level balance for
// the equity numerator (acceptable approximation for the v0.4 single-position UI).
function computeMarginRatio(
  position: Position,
  markPrice: number | undefined,
  balance: number | undefined,
): number | null {
  const size = parseFloat(position.size);
  if (!size) return null;
  const upnl = parseFloat(position.upnl);
  const mp = markPrice ?? parseFloat(position.avg_entry);
  if (!mp) return null;
  const notional = size * mp;
  if (notional <= 0) return null;
  const equity = (balance ?? 0) + upnl;
  return equity / notional;
}

export default function Positions({ position, markPrice, balance }: PositionsProps) {
  const ratio = position ? computeMarginRatio(position, markPrice, balance) : null;
  const danger = ratio !== null && ratio < WARN;
  const critical = ratio !== null && ratio < MMR;

  const rowBorder = critical
    ? 'border-2 border-red-500 animate-pulse'
    : danger
      ? 'border border-red-500 animate-pulse'
      : 'border border-gray-800';

  return (
    <div className="flex flex-col p-3">
      <div className="text-xs text-gray-400 mb-3 border-b border-gray-800 pb-2">
        Open Position
      </div>

      {!position || parseFloat(position.size) === 0 ? (
        <div className="text-xs text-gray-600 text-center py-4">No open position</div>
      ) : (
        <div className={`rounded p-3 bg-[#111] ${rowBorder}`}>
          <div className="flex justify-between items-center mb-2">
            <span className="text-xs text-gray-400 font-mono">{position.symbol}</span>
            <span
              className={`text-xs font-semibold px-2 py-0.5 rounded ${
                position.side === 'buy'
                  ? 'bg-[#26a69a]/20 text-[#26a69a]'
                  : 'bg-[#ef5350]/20 text-[#ef5350]'
              }`}
            >
              {position.side === 'buy' ? 'LONG' : 'SHORT'}
            </span>
          </div>
          <div className="grid grid-cols-2 gap-y-2 text-xs font-mono">
            <div className="text-gray-500">Size</div>
            <div className={`text-right ${danger ? 'text-red-400' : 'text-white'}`}>
              {parseFloat(position.size).toFixed(4)}
            </div>
            <div className="text-gray-500">Avg Entry</div>
            <div className={`text-right ${danger ? 'text-red-400' : 'text-white'}`}>
              {parseFloat(position.avg_entry).toFixed(2)}
            </div>
            <div className="text-gray-500">UPnL</div>
            <div
              className={`text-right font-semibold ${
                parseFloat(position.upnl) >= 0 ? 'text-[#26a69a]' : 'text-[#ef5350]'
              }`}
            >
              {parseFloat(position.upnl) >= 0 ? '+' : ''}
              {parseFloat(position.upnl).toFixed(4)}
            </div>
            {ratio !== null && (
              <>
                <div className="text-gray-500">Margin Ratio</div>
                <div
                  className={`text-right font-semibold ${
                    critical
                      ? 'text-red-500'
                      : danger
                        ? 'text-red-400'
                        : 'text-white'
                  }`}
                >
                  {(ratio * 100).toFixed(2)}%
                </div>
              </>
            )}
          </div>
          {danger && (
            <div className="mt-2 text-xs text-red-400 text-center font-bold">
              {critical ? '⚠ LIQUIDATION IMMINENT' : '⚠ MARGIN LOW'}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
