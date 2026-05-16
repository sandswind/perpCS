'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
import { WS_BASE } from '@/lib/config';
import type { SessionCloseEvent } from '@/components/SessionEndModal';

export interface Position {
  symbol: string;
  side: string;
  size: string;
  avg_entry: string;
  upnl: string;
}

export interface Fill {
  order_id: number;
  price: string;
  qty: string;
  pnl: string;
}

// LiquidationEvent matches the server-side fanout.liquidationMsg payload.
export interface LiquidationEvent {
  symbol: string;
  size: string;
  mark_price: string;
  loss: string;
  ts: number;
}

// FundingEvent matches the server-side fanout.fundingMsg payload.
export interface FundingEvent {
  rate: number;
  ts: number;
}

interface WSEnvelope {
  type: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  data: any;
}

export function useAccountWS(sessionId: string) {
  const [position, setPosition] = useState<Position | null>(null);
  const [fills, setFills] = useState<Fill[]>([]);
  const [liquidation, setLiquidation] = useState<LiquidationEvent | null>(null);
  const [fundingRate, setFundingRate] = useState<number | null>(null);
  const [lastFundingTS, setLastFundingTS] = useState<number | null>(null);
  const [sessionClose, setSessionClose] = useState<SessionCloseEvent | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    const ws = new WebSocket(`${WS_BASE}/ws/account/${sessionId}`);
    wsRef.current = ws;

    ws.onmessage = (event) => {
      try {
        const envelope: WSEnvelope = JSON.parse(event.data);

        if (envelope.type === 'position') {
          setPosition(envelope.data as Position);
        } else if (envelope.type === 'fill') {
          const fill = envelope.data as Fill;
          setFills((prev) => [fill, ...prev].slice(0, 50));
        } else if (envelope.type === 'liquidation') {
          setLiquidation(envelope.data as LiquidationEvent);
        } else if (envelope.type === 'funding') {
          const f = envelope.data as FundingEvent;
          setFundingRate(f.rate);
          setLastFundingTS(f.ts);
        } else if (envelope.type === 'session_close') {
          // v0.6: replay ended → trigger settlement modal
          setSessionClose(envelope.data as SessionCloseEvent);
        }
      } catch {
        // ignore
      }
    };

    ws.onclose = () => {
      reconnectRef.current = setTimeout(() => {
        connect();
      }, 2000);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, [sessionId]);

  useEffect(() => {
    connect();
    return () => {
      if (reconnectRef.current) clearTimeout(reconnectRef.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return { position, fills, liquidation, fundingRate, lastFundingTS, sessionClose };
}
