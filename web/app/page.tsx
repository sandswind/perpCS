'use client';

import { useEffect, useState } from 'react';
import dynamic from 'next/dynamic';
import { useMarketWS } from '@/hooks/useMarketWS';
import { useAccountWS } from '@/hooks/useAccountWS';
import OrderBook from '@/components/OrderBook';
import OrderForm from '@/components/OrderForm';
import Positions from '@/components/Positions';
import TradeHistory from '@/components/TradeHistory';
import LiquidationModal from '@/components/LiquidationModal';
import { SESSION_ID, API_BASE } from '@/lib/config';

// Dynamic import for Chart to avoid SSR issues with DOM APIs
const Chart = dynamic(() => import('@/components/Chart'), { ssr: false });

const SYMBOL = 'BTC-MED';

export default function TradingPage() {
  const { orderbook, klineData } = useMarketWS(SYMBOL);
  const { position, fills, liquidation, fundingRate } = useAccountWS(SESSION_ID);
  const [closedLiq, setClosedLiq] = useState(false);
  const [balance, setBalance] = useState<number | undefined>(undefined);

  // Reset the modal-close flag whenever a new liquidation event arrives.
  useEffect(() => {
    if (liquidation) setClosedLiq(false);
  }, [liquidation]);

  // Periodically poll /account so we can show balance + driver margin ratio.
  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const r = await fetch(`${API_BASE}/account`);
        if (!r.ok) return;
        const j = await r.json();
        if (!cancelled) setBalance(parseFloat(j.balance));
      } catch {
        // ignore network blips
      }
    };
    tick();
    const id = setInterval(tick, 2000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, []);

  // Mid price from the order book = our best on-frontend mark-price proxy.
  const markPrice =
    orderbook.bids.length && orderbook.asks.length
      ? (parseFloat(orderbook.bids[0].price) + parseFloat(orderbook.asks[0].price)) / 2
      : undefined;

  return (
    <div
      className="flex h-screen w-screen overflow-hidden font-sans"
      style={{ backgroundColor: '#0f0f0f', color: '#d1d4dc' }}
    >
      {/* Left panel: Chart + OrderBook */}
      <div className="flex flex-col flex-1 min-w-0 border-r border-gray-800">
        {/* Chart */}
        <div className="flex-1 min-h-0 border-b border-gray-800" style={{ background: '#1a1a1a' }}>
          <Chart klineData={klineData} symbol={SYMBOL} />
        </div>
        {/* Order Book */}
        <div className="h-80 flex-shrink-0" style={{ background: '#1a1a1a' }}>
          <OrderBook orderbook={orderbook} symbol={SYMBOL} />
        </div>
      </div>

      {/* Middle panel: Order Form */}
      <div className="w-64 flex-shrink-0 border-r border-gray-800" style={{ background: '#1a1a1a' }}>
        <OrderForm symbol={SYMBOL} />
        {fundingRate !== null && (
          <div className="px-3 py-2 text-xs text-gray-500 border-t border-gray-800 font-mono">
            funding: {(fundingRate * 100).toFixed(4)}%
          </div>
        )}
      </div>

      {/* Right panel: Positions + Trade History */}
      <div className="w-72 flex-shrink-0 flex flex-col" style={{ background: '#1a1a1a' }}>
        <Positions position={position} markPrice={markPrice} balance={balance} />
        <div className="flex-1 border-t border-gray-800 overflow-hidden flex flex-col">
          <TradeHistory fills={fills} />
        </div>
      </div>

      {/* Full-screen liquidation overlay */}
      {liquidation && !closedLiq && (
        <LiquidationModal event={liquidation} onClose={() => setClosedLiq(true)} />
      )}
    </div>
  );
}
