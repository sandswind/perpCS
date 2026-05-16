'use client';

import { useEffect, useState } from 'react';
import { API_BASE } from '@/lib/config';

export interface PnLPoint {
  ts: number;   // chaos clock ns
  pnl: number;  // cumulative realized PnL in USDR
}

export interface WithdrawReceipt {
  session_id: string;
  player: string;
  final_equity: string;
  final_equity_raw: number;
  nonce: number;
  chain_id: number;
  signature: string;
  signer: string;
  archive_path?: string;
}

export interface SessionReport {
  receipt: WithdrawReceipt | null;
  total_trades: number;
  realized_pnl: string;
  max_drawdown: string;
  pnl_curve: PnLPoint[];
  was_liquidated: boolean;
}

interface UseReportResult {
  report: SessionReport | null;
  loading: boolean;
  error: string | null;
}

export function useReport(address: string, sessionId: string): UseReportResult {
  const [report, setReport] = useState<SessionReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!address && !sessionId) {
      setLoading(false);
      return;
    }

    let cancelled = false;
    const addr = address || 'player1';

    // First ensure the session is settled, then fetch the report.
    const fetchReport = async () => {
      try {
        // Try to settle (idempotent)
        await fetch(`${API_BASE}/sessions/${addr}/close`, { method: 'POST' });

        // Fetch the report
        const res = await fetch(`${API_BASE}/sessions/${addr}/report`);
        if (!res.ok) {
          if (res.status === 404) {
            throw new Error('Report not ready yet — session may still be running.');
          }
          const body = await res.json().catch(() => ({})) as { error?: string };
          throw new Error(body.error || `HTTP ${res.status}`);
        }
        const data: SessionReport = await res.json();
        if (!cancelled) {
          setReport(data);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setError(String(err instanceof Error ? err.message : err));
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    fetchReport();
    return () => {
      cancelled = true;
    };
  }, [address, sessionId]);

  return { report, loading, error };
}
