'use client';

import type { OrderBook as OrderBookType, PriceLevel } from '@/hooks/useMarketWS';

interface OrderBookProps {
  orderbook: OrderBookType;
  symbol: string;
}

function calcMaxQty(levels: PriceLevel[]): number {
  return Math.max(...levels.map((l) => parseFloat(l.qty)), 0.0001);
}

function Row({
  level,
  maxQty,
  side,
}: {
  level: PriceLevel;
  maxQty: number;
  side: 'bid' | 'ask';
}) {
  const qty = parseFloat(level.qty);
  const pct = Math.min((qty / maxQty) * 100, 100);
  const color = side === 'bid' ? '#26a69a' : '#ef5350';
  const bgColor = side === 'bid' ? 'rgba(38,166,154,0.12)' : 'rgba(239,83,80,0.12)';

  return (
    <div className="relative flex justify-between px-2 py-[2px] text-xs font-mono">
      <div
        className="absolute inset-y-0 right-0"
        style={{ width: `${pct}%`, backgroundColor: bgColor }}
      />
      <span style={{ color }} className="relative z-10">
        {parseFloat(level.price).toFixed(1)}
      </span>
      <span className="relative z-10 text-gray-300">
        {parseFloat(level.qty).toFixed(4)}
      </span>
    </div>
  );
}

export default function OrderBook({ orderbook, symbol }: OrderBookProps) {
  const maxBidQty = calcMaxQty(orderbook.bids);
  const maxAskQty = calcMaxQty(orderbook.asks);

  const spreadNum =
    orderbook.asks.length > 0 && orderbook.bids.length > 0
      ? parseFloat(orderbook.asks[0].price) - parseFloat(orderbook.bids[0].price)
      : 0;

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <div className="px-3 py-2 text-xs text-gray-400 border-b border-gray-800">
        {symbol} Order Book
        {spreadNum > 0 && (
          <span className="ml-2 text-gray-500">
            Spread: {spreadNum.toFixed(1)}
          </span>
        )}
      </div>
      {/* Header */}
      <div className="flex justify-between px-2 py-1 text-xs text-gray-500 font-mono border-b border-gray-800">
        <span>Price</span>
        <span>Qty</span>
      </div>
      {/* Asks (reversed — highest ask at top) */}
      <div className="flex-1 flex flex-col-reverse overflow-hidden">
        {[...orderbook.asks].reverse().map((level, i) => (
          <Row key={`ask-${i}`} level={level} maxQty={maxAskQty} side="ask" />
        ))}
      </div>
      {/* Mid price */}
      {orderbook.bids.length > 0 && orderbook.asks.length > 0 && (
        <div className="px-2 py-1 text-center text-sm font-mono font-semibold text-yellow-400 border-y border-gray-800">
          {(
            (parseFloat(orderbook.bids[0].price) +
              parseFloat(orderbook.asks[0].price)) /
            2
          ).toFixed(1)}
        </div>
      )}
      {/* Bids */}
      <div className="flex-1 overflow-hidden">
        {orderbook.bids.map((level, i) => (
          <Row key={`bid-${i}`} level={level} maxQty={maxBidQty} side="bid" />
        ))}
      </div>
    </div>
  );
}
