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
import SessionEndModal, { type SessionCloseEvent } from '@/components/SessionEndModal';
import { SESSION_ID as DEFAULT_SESSION_ID, API_BASE } from '@/lib/config';

// Dynamic import for Chart to avoid SSR issues with DOM APIs
const Chart = dynamic(() => import('@/components/Chart'), { ssr: false });

const SYMBOL = 'BTC-MED';
const DEFAULT_BALANCE = 10_000;

function TradeView() {
  const sp = useSearchParams();
  const sessionId = sp.get('session') || DEFAULT_SESSION_ID;
  const address = sp.get('address') || '';
  const accountQuery = address ? `?address=${address}` : '';

  const { orderbook, klineData } = useMarketWS(SYMBOL);
  const { position, fills, liquidation, fundingRate, sessionClose: wsSessionClose } = useAccountWS(sessionId);
  const [closedLiq, setClosedLiq] = useState(false);
  const [balance, setBalance] = useState<number | undefined>(undefined);
  const [initialBalance, setInitialBalance] = useState(DEFAULT_BALANCE);
  const [sessionCloseEvent, setSessionCloseEvent] = useState<SessionCloseEvent | null>(null);
  const [sessionEndDismissed, setSessionEndDismissed] = useState(false);

  // Use WS session_close event if available
  useEffect(() => {
    if (wsSessionClose && !sessionCloseEvent) {
      setSessionCloseEvent(wsSessionClose);
    }
  }, [wsSessionClose, sessionCloseEvent]);

  useEffect(() => {
    if (liquidation) setClosedLiq(false);
  }, [liquidation]);

  // Periodically poll /account so we can show balance + drive margin ratio.
  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const r = await fetch(`${API_BASE}/account${accountQuery}`);
        if (!r.ok) return;
        const j = await r.json();
        if (!cancelled) {
          const b = parseFloat(j.balance);
          setBalance(b);
          // Capture initial balance on first successful poll
          setInitialBalance((prev) => (prev === DEFAULT_BALANCE && b > 0 ? b : prev));
        }
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

  // Listen for session_close events from the account WS (v0.6)
  // The useAccountWS hook returns a `sessionClose` field — we extend it below.
  // For now, poll for a closed session every 5s as a fallback.
  useEffect(() => {
    if (sessionEndDismissed || sessionCloseEvent) return;
    let cancelled = false;
    const addr = address || 'player1';
    const check = async () => {
      try {
        const res = await fetch(`${API_BASE}/sessions/${addr}/receipt`);
        if (res.ok) {
          const rec = await res.json();
          if (!cancelled && rec?.final_equity) {
            setSessionCloseEvent({
              session_id: rec.session_id || sessionId,
              final_equity: rec.final_equity,
              ts: Date.now(),
            });
          }
        }
      } catch {
        // ignore — 404 is normal while session running
      }
    };
    const id = setInterval(check, 5000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [address, sessionId, sessionEndDismissed, sessionCloseEvent]);

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
          <div className="flex items-center gap-3">
            <span className="text-gray-500 font-mono">
              {address ? `${address.slice(0, 6)}…${address.slice(-4)}` : 'demo'}
              {' · '}
              <span className="text-gray-600">{sessionId.slice(0, 14)}…</span>
            </span>
            {/* Manual "End Session" button */}
            <button
              onClick={() => {
                const addr = address || 'player1';
                fetch(`${API_BASE}/sessions/${addr}/close`, { method: 'POST' })
                  .then((r) => r.json())
                  .then((rec) => {
                    setSessionCloseEvent({
                      session_id: rec.session_id || sessionId,
                      final_equity: rec.final_equity || '0',
                      ts: Date.now(),
                    });
                  })
                  .catch(() => {});
              }}
              className="px-2 py-0.5 rounded text-xs border border-gray-700 text-gray-500 hover:text-red-400 hover:border-red-700 transition-colors"
            >
              End Session
            </button>
          </div>
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

      {/* v0.6: Session-end modal (appears when replay ends or player clicks End Session) */}
      {sessionCloseEvent && !sessionEndDismissed && (
        <SessionEndModal
          event={sessionCloseEvent}
          address={address || 'player1'}
          sessionId={sessionId}
          initialBalance={initialBalance}
          onClose={() => setSessionEndDismissed(true)}
        />
      )}
    </div>
  );
}

export default function TradingPage() {
  return (
    <Suspense fallback={<div className="p-8 text-gray-500">Loading…</div>}>
      <TradeView />
    </Suspense>
  );
}
