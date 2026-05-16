'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
import { WS_BASE } from '@/lib/config';

export interface PriceLevel {
  price: string;
  qty: string;
}

export interface OrderBook {
  bids: PriceLevel[];
  asks: PriceLevel[];
}

export interface Trade {
  price: string;
  qty: string;
  side: string;
  ts: number;
}

export interface CandleBar {
  time: number; // unix seconds
  open: number;
  high: number;
  low: number;
  close: number;
}

interface BookSnapshotMsg {
  symbol: string;
  ts: number;
  bids: PriceLevel[];
  asks: PriceLevel[];
}

interface WSEnvelope {
  type: string;
  data: BookSnapshotMsg | Trade;
}

export function useMarketWS(symbol: string) {
  const [orderbook, setOrderbook] = useState<OrderBook>({ bids: [], asks: [] });
  const [lastTrade, setLastTrade] = useState<Trade | null>(null);
  const [klineData, setKlineData] = useState<CandleBar[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const currentBarRef = useRef<CandleBar | null>(null);

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    const ws = new WebSocket(`${WS_BASE}/ws/market/${symbol}`);
    wsRef.current = ws;

    ws.onmessage = (event) => {
      try {
        const envelope: WSEnvelope = JSON.parse(event.data);

        if (envelope.type === 'book_snapshot') {
          const snap = envelope.data as BookSnapshotMsg;
          setOrderbook({
            bids: snap.bids.slice(0, 20),
            asks: snap.asks.slice(0, 20),
          });

          // Build fake 1-second candles from mid price
          if (snap.bids.length > 0 && snap.asks.length > 0) {
            const midPrice =
              (parseFloat(snap.bids[0].price) + parseFloat(snap.asks[0].price)) / 2;
            const barTime = Math.floor(snap.ts / 1_000_000_000); // ns → s

            setKlineData((prev) => {
              const bar = currentBarRef.current;
              if (!bar || bar.time !== barTime) {
                // New bar
                const newBar: CandleBar = {
                  time: barTime,
                  open: midPrice,
                  high: midPrice,
                  low: midPrice,
                  close: midPrice,
                };
                currentBarRef.current = newBar;
                // Avoid duplicate times
                const filtered = prev.filter((b) => b.time !== barTime);
                return [...filtered, newBar].slice(-200);
              } else {
                // Update existing bar
                const updated: CandleBar = {
                  ...bar,
                  high: Math.max(bar.high, midPrice),
                  low: Math.min(bar.low, midPrice),
                  close: midPrice,
                };
                currentBarRef.current = updated;
                const filtered = prev.filter((b) => b.time !== barTime);
                return [...filtered, updated].slice(-200);
              }
            });
          }
        } else if (envelope.type === 'trade') {
          setLastTrade(envelope.data as Trade);
        }
      } catch {
        // ignore parse errors
      }
    };

    ws.onclose = () => {
      // Reconnect after 2s
      reconnectRef.current = setTimeout(() => {
        connect();
      }, 2000);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, [symbol]);

  useEffect(() => {
    connect();
    return () => {
      if (reconnectRef.current) clearTimeout(reconnectRef.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return { orderbook, lastTrade, klineData };
}
