'use client';

import { useEffect, useRef, useState, useCallback } from 'react';
import { WS_BASE } from '@/lib/config';

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

interface WSEnvelope {
  type: string;
  data: Position | Fill;
}

export function useAccountWS(sessionId: string) {
  const [position, setPosition] = useState<Position | null>(null);
  const [fills, setFills] = useState<Fill[]>([]);
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

  return { position, fills };
}
