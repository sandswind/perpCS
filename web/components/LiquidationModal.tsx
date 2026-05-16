'use client';

import type { LiquidationEvent } from '@/hooks/useAccountWS';

interface Props {
  event: LiquidationEvent | null;
  onClose: () => void;
}

// LiquidationModal is a full-screen overlay shown when the player's position
// is force-closed. Pure CSS / no DOM-API usage at module top level, so SSR
// works without dynamic import.
export default function LiquidationModal({ event, onClose }: Props) {
  if (!event) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black bg-opacity-80"
      role="dialog"
      aria-modal="true"
    >
      <div className="bg-red-900 border-2 border-red-500 p-8 rounded-xl text-center max-w-md mx-4 shadow-2xl">
        <div className="text-6xl mb-4">💀</div>
        <h2 className="text-3xl font-bold text-red-200">YOU GOT LIQUIDATED</h2>
        <p className="mt-4 text-gray-200">
          Position <span className="font-mono">{event.symbol}</span> closed at mark{' '}
          <span className="font-mono text-red-300">
            {parseFloat(event.mark_price).toFixed(2)}
          </span>
        </p>
        <p className="text-gray-300 mt-2">
          Insurance fund covered{' '}
          <span className="font-mono text-yellow-300">
            {parseFloat(event.loss).toFixed(4)} USDR
          </span>{' '}
          deficit
        </p>
        <p className="mt-4 text-sm text-gray-400 italic">
          Tip: lower leverage or set a stop next time.
        </p>
        <button
          onClick={onClose}
          className="mt-6 px-6 py-2 bg-red-600 hover:bg-red-700 rounded font-semibold text-white"
        >
          Acknowledge
        </button>
      </div>
    </div>
  );
}
