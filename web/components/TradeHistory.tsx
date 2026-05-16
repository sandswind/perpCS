'use client';

import type { Fill } from '@/hooks/useAccountWS';

interface TradeHistoryProps {
  fills: Fill[];
}

export default function TradeHistory({ fills }: TradeHistoryProps) {
  return (
    <div className="flex flex-col flex-1 overflow-hidden p-3">
      <div className="text-xs text-gray-400 mb-3 border-b border-gray-800 pb-2">
        Trade History
      </div>

      {fills.length === 0 ? (
        <div className="text-xs text-gray-600 text-center py-4">No fills yet</div>
      ) : (
        <div className="overflow-y-auto flex-1">
          <table className="w-full text-xs font-mono">
            <thead>
              <tr className="text-gray-500 border-b border-gray-800">
                <th className="text-left pb-1 font-normal">Order</th>
                <th className="text-right pb-1 font-normal">Price</th>
                <th className="text-right pb-1 font-normal">Qty</th>
                <th className="text-right pb-1 font-normal">PnL</th>
              </tr>
            </thead>
            <tbody>
              {fills.map((fill, i) => {
                const pnl = parseFloat(fill.pnl);
                return (
                  <tr key={`fill-${i}-${fill.order_id}`} className="border-b border-gray-800/50">
                    <td className="py-1 text-gray-400">#{fill.order_id}</td>
                    <td className="py-1 text-right text-white">
                      {parseFloat(fill.price).toFixed(1)}
                    </td>
                    <td className="py-1 text-right text-gray-300">
                      {parseFloat(fill.qty).toFixed(4)}
                    </td>
                    <td
                      className={`py-1 text-right ${
                        pnl >= 0 ? 'text-[#26a69a]' : 'text-[#ef5350]'
                      }`}
                    >
                      {pnl >= 0 ? '+' : ''}
                      {pnl.toFixed(2)}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
