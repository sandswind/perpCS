'use client';

import dynamic from 'next/dynamic';
import { useMarketWS } from '@/hooks/useMarketWS';
import { useAccountWS } from '@/hooks/useAccountWS';
import OrderBook from '@/components/OrderBook';
import OrderForm from '@/components/OrderForm';
import Positions from '@/components/Positions';
import TradeHistory from '@/components/TradeHistory';
import { SESSION_ID } from '@/lib/config';

// Dynamic import for Chart to avoid SSR issues with DOM APIs
const Chart = dynamic(() => import('@/components/Chart'), { ssr: false });

const SYMBOL = 'BTC-MED';

export default function TradingPage() {
  const { orderbook, klineData } = useMarketWS(SYMBOL);
  const { position, fills } = useAccountWS(SESSION_ID);

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
      </div>

      {/* Right panel: Positions + Trade History */}
      <div className="w-72 flex-shrink-0 flex flex-col" style={{ background: '#1a1a1a' }}>
        <Positions position={position} />
        <div className="flex-1 border-t border-gray-800 overflow-hidden flex flex-col">
          <TradeHistory fills={fills} />
        </div>
      </div>
    </div>
  );
}
