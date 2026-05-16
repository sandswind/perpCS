'use client';

import type { Position } from '@/hooks/useAccountWS';

interface PositionsProps {
  position: Position | null;
}

export default function Positions({ position }: PositionsProps) {
  return (
    <div className="flex flex-col p-3">
      <div className="text-xs text-gray-400 mb-3 border-b border-gray-800 pb-2">
        Open Position
      </div>

      {!position || parseFloat(position.size) === 0 ? (
        <div className="text-xs text-gray-600 text-center py-4">No open position</div>
      ) : (
        <div className="rounded border border-gray-800 p-3 bg-[#111]">
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
            <div className="text-right text-white">{parseFloat(position.size).toFixed(4)}</div>
            <div className="text-gray-500">Avg Entry</div>
            <div className="text-right text-white">{parseFloat(position.avg_entry).toFixed(2)}</div>
            <div className="text-gray-500">UPnL</div>
            <div
              className={`text-right font-semibold ${
                parseFloat(position.upnl) >= 0 ? 'text-[#26a69a]' : 'text-[#ef5350]'
              }`}
            >
              {parseFloat(position.upnl) >= 0 ? '+' : ''}
              {parseFloat(position.upnl).toFixed(4)}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
