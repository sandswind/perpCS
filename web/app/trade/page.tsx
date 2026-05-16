'use client';

import { useEffect, useState, Suspense } from 'react';
import dynamic from 'next/dynamic';
import { useSearchParams } from 'next/navigation';
import Link from 'next/link';
import { useMarketWS } from '@/hooks/useMarketWS';
import { useAccountWS } from '@/hooks/useAccountWS';
import OrderBook from '@/components/OrderBook';
import OrderForm from '@/components/OrderForm';
import Positions from '@/components/Positions';
import TradeHistory from '@/components/TradeHistory';
import LiquidationModal from '@/components/LiquidationModal';
import { SESSION_ID as DEFAULT_SESSION_ID, API_BASE } from '@/lib/config';

// Dynamic import for Chart to avoid SSR issues with DOM APIs
const Chart = dynamic(() => import('@/components/Chart'), { ssr: false });

const SYMBOL = 'BTC-MED';

function TradeView() {
  const sp = useSearchParams();
  // v0.5: when arriving from the on-chain entry flow, the URL carries
  // ?session=0x... (sessionId from GameVault) and ?address=0x... (wallet).
  // For the v0.4 demo path we fall back to a hard-coded session.
  const sessionId = sp.get('session') || DEFAULT_SESSION_ID;
  const address = sp.get('address') || '';
  const accountQuery = address ? `?address=${address}` : '';

  const { orderbook, klineData } = useMarketWS(SYMBOL);
  const { position, fills, liquidation, fundingRate } = useAccountWS(sessionId);
  const [closedLiq, setClosedLiq] = useState(false);
  const [balance, setBalance] = useState<number | undefined>(undefined);

  useEffect(() => {
    if (liquidation) setClosedLiq(false);
  }, [liquidation]);

  // Periodically poll /account so we can show balance + driver margin ratio.
  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const r = await fetch(`${API_BASE}/account${accountQuery}`);
        if (!r.ok) return;
        const j = await r.json();
        if (!cancelled) setBalance(parseFloat(j.balance));
      } catch {
        // ignore
      }
    };
    tick();
    const id = setInterval(tick, 2000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [accountQuery]);

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
        <div className="px-3 py-1.5 border-b border-gray-800 text-xs flex items-center justify-between" style={{ background: '#1a1a1a' }}>
          <Link href="/" className="text-gray-500 hover:text-white">← Lobby</Link>
          <span className="text-gray-500 font-mono">
            {address ? `${address.slice(0, 6)}…${address.slice(-4)}` : 'demo'}
            {' · '}
            <span className="text-gray-600">{sessionId.slice(0, 14)}…</span>
          </span>
        </div>
        <div className="flex-1 min-h-0 border-b border-gray-800" style={{ background: '#1a1a1a' }}>
          <Chart klineData={klineData} symbol={SYMBOL} />
        </div>
        <div className="h-80 flex-shrink-0" style={{ background: '#1a1a1a' }}>
          <OrderBook orderbook={orderbook} symbol={SYMBOL} />
        </div>
      </div>

      <div className="w-64 flex-shrink-0 border-r border-gray-800" style={{ background: '#1a1a1a' }}>
        <OrderForm symbol={SYMBOL} />
        {fundingRate !== null && (
          <div className="px-3 py-2 text-xs text-gray-500 border-t border-gray-800 font-mono">
            funding: {(fundingRate * 100).toFixed(4)}%
          </div>
        )}
      </div>

      <div className="w-72 flex-shrink-0 flex flex-col" style={{ background: '#1a1a1a' }}>
        <Positions position={position} markPrice={markPrice} balance={balance} />
        <div className="flex-1 border-t border-gray-800 overflow-hidden flex flex-col">
          <TradeHistory fills={fills} />
        </div>
      </div>

      {liquidation && !closedLiq && (
        <LiquidationModal event={liquidation} onClose={() => setClosedLiq(true)} />
      )}
    </div>
  );
}

export default function TradingPage() {
  // useSearchParams must be inside Suspense per Next.js 14.
  return (
    <Suspense fallback={<div className="p-8 text-gray-500">Loading…</div>}>
      <TradeView />
    </Suspense>
  );
}
