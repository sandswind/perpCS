'use client';

// PnLChart — SVG-based PnL curve, zero dependencies.
// Renders the cumulative realized PnL over chaos-clock time.
// Points with ts=0 are skipped. Negative PnL area is red, positive green.

import { useMemo } from 'react';
import type { PnLPoint } from '@/hooks/useReport';

interface Props {
  data: PnLPoint[];
  height?: number;
}

export default function PnLChart({ data, height = 200 }: Props) {
  const WIDTH = 800;
  const PAD_TOP = 16;
  const PAD_RIGHT = 12;
  const PAD_BOTTOM = 28;
  const PAD_LEFT = 60;

  const filtered = useMemo(() => data.filter((p) => p.ts > 0), [data]);

  const { minPnL, maxPnL, minTS, maxTS, points, zeroY } = useMemo(() => {
    if (filtered.length < 2) {
      return { minPnL: 0, maxPnL: 1, minTS: 0, maxTS: 1, points: [], zeroY: height / 2 };
    }
    const pnls = filtered.map((p) => p.pnl);
    const tss = filtered.map((p) => p.ts);
    const minPnL = Math.min(...pnls);
    const maxPnL = Math.max(...pnls);
    const minTS = Math.min(...tss);
    const maxTS = Math.max(...tss);

    const pnlRange = maxPnL - minPnL || 1;
    const tsRange = maxTS - minTS || 1;
    const chartW = WIDTH - PAD_LEFT - PAD_RIGHT;
    const chartH = height - PAD_TOP - PAD_BOTTOM;

    const toX = (ts: number) => PAD_LEFT + ((ts - minTS) / tsRange) * chartW;
    const toY = (pnl: number) => PAD_TOP + ((maxPnL - pnl) / pnlRange) * chartH;

    const points = filtered.map((p) => ({ x: toX(p.ts), y: toY(p.pnl), pnl: p.pnl }));
    const zeroY = toY(0);

    return { minPnL, maxPnL, minTS, maxTS, points, zeroY };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filtered, height]);

  if (filtered.length < 2) {
    return (
      <div
        className="flex items-center justify-center text-xs text-gray-600"
        style={{ height }}
      >
        Not enough data points
      </div>
    );
  }

  // Build SVG polyline path
  const linePath = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');

  // Fill area under line (closed at the zero baseline)
  const first = points[0];
  const last = points[points.length - 1];
  const fillPath = `${linePath} L${last.x.toFixed(1)},${zeroY.toFixed(1)} L${first.x.toFixed(1)},${zeroY.toFixed(1)} Z`;

  // Y-axis labels (5 ticks)
  const pnlRange = maxPnL - minPnL || 1;
  const yTicks = Array.from({ length: 5 }, (_, i) => {
    const val = minPnL + (pnlRange * (4 - i)) / 4;
    const chartH = height - PAD_TOP - PAD_BOTTOM;
    const y = PAD_TOP + (i / 4) * chartH;
    return { val, y };
  });

  // X-axis labels (3 ticks: start, mid, end)
  const tsRange = maxTS - minTS || 1;
  const xTicks = [0, 0.5, 1].map((frac) => {
    const ts = minTS + tsRange * frac;
    const chartW = WIDTH - PAD_LEFT - PAD_RIGHT;
    const x = PAD_LEFT + frac * chartW;
    const seconds = Math.round((ts - minTS) / 1e9);
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const label = `+${h}h${m.toString().padStart(2, '0')}m`;
    return { x, label };
  });

  const lineColor = last.pnl >= 0 ? '#26a69a' : '#ef5350';
  const fillColor = last.pnl >= 0 ? '#26a69a22' : '#ef535022';

  return (
    <svg
      viewBox={`0 0 ${WIDTH} ${height}`}
      style={{ width: '100%', height }}
      preserveAspectRatio="none"
    >
      {/* Grid lines */}
      {yTicks.map((t, i) => (
        <line key={i} x1={PAD_LEFT} y1={t.y} x2={WIDTH - PAD_RIGHT} y2={t.y} stroke="#1e1e1e" strokeWidth="1" />
      ))}

      {/* Zero line */}
      <line x1={PAD_LEFT} y1={zeroY} x2={WIDTH - PAD_RIGHT} y2={zeroY} stroke="#333" strokeWidth="1" strokeDasharray="4 3" />

      {/* Fill + line */}
      <path d={fillPath} fill={fillColor} />
      <path d={linePath} fill="none" stroke={lineColor} strokeWidth="1.5" />

      {/* Y-axis labels */}
      {yTicks.map((t, i) => (
        <text key={i} x={PAD_LEFT - 6} y={t.y + 4} textAnchor="end" fontSize="9" fill="#666" fontFamily="monospace">
          {t.val >= 0 ? '+' : ''}{t.val.toFixed(0)}
        </text>
      ))}

      {/* X-axis labels */}
      {xTicks.map((t, i) => (
        <text key={i} x={t.x} y={height - 6} textAnchor="middle" fontSize="9" fill="#555" fontFamily="monospace">
          {t.label}
        </text>
      ))}
    </svg>
  );
}
