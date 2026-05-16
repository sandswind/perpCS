'use client';

import { useState } from 'react';
import { placeOrder } from '@/lib/api';

interface OrderFormProps {
  symbol: string;
}

export default function OrderForm({ symbol }: OrderFormProps) {
  const [side, setSide] = useState<'buy' | 'sell'>('buy');
  const [type, setType] = useState<'market' | 'limit'>('market');
  const [qty, setQty] = useState('0.1');
  const [price, setPrice] = useState('');
  const [status, setStatus] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setStatus(null);
    try {
      const result = (await placeOrder(side, type, qty, type === 'limit' ? price : undefined)) as {
        order_id?: number;
        trades?: Array<{ price: string; quantity: string }>;
        error?: string;
      };
      if (result.error) {
        setStatus(`Error: ${result.error}`);
      } else {
        const fills = result.trades?.length ?? 0;
        setStatus(
          fills > 0
            ? `Filled ${fills} trade(s) @ ${result.trades![0].price}`
            : `Order placed (id: ${result.order_id})`
        );
      }
    } catch (err) {
      setStatus(`Network error: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="flex flex-col h-full p-3">
      <div className="text-xs text-gray-400 mb-3 border-b border-gray-800 pb-2">
        Place Order · {symbol}
      </div>

      {/* Side tabs */}
      <div className="flex rounded overflow-hidden mb-3 border border-gray-700">
        <button
          className={`flex-1 py-2 text-sm font-semibold transition-colors ${
            side === 'buy'
              ? 'bg-[#26a69a] text-black'
              : 'bg-transparent text-gray-400 hover:text-white'
          }`}
          onClick={() => setSide('buy')}
          type="button"
        >
          Buy / Long
        </button>
        <button
          className={`flex-1 py-2 text-sm font-semibold transition-colors ${
            side === 'sell'
              ? 'bg-[#ef5350] text-black'
              : 'bg-transparent text-gray-400 hover:text-white'
          }`}
          onClick={() => setSide('sell')}
          type="button"
        >
          Sell / Short
        </button>
      </div>

      {/* Type selector */}
      <div className="flex gap-2 mb-3">
        {(['market', 'limit'] as const).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setType(t)}
            className={`px-3 py-1 text-xs rounded border transition-colors ${
              type === t
                ? 'border-gray-400 text-white bg-gray-700'
                : 'border-gray-700 text-gray-500 hover:text-gray-300'
            }`}
          >
            {t.charAt(0).toUpperCase() + t.slice(1)}
          </button>
        ))}
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-3">
        {/* Price (limit only) */}
        {type === 'limit' && (
          <div>
            <label className="block text-xs text-gray-500 mb-1">Price (USDR)</label>
            <input
              type="number"
              step="0.1"
              min="0"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              className="w-full bg-[#0f0f0f] border border-gray-700 rounded px-3 py-2 text-sm font-mono text-white focus:outline-none focus:border-gray-400"
              placeholder="0.0"
              required
            />
          </div>
        )}

        {/* Quantity */}
        <div>
          <label className="block text-xs text-gray-500 mb-1">Quantity (BTC)</label>
          <input
            type="number"
            step="0.01"
            min="0.01"
            value={qty}
            onChange={(e) => setQty(e.target.value)}
            className="w-full bg-[#0f0f0f] border border-gray-700 rounded px-3 py-2 text-sm font-mono text-white focus:outline-none focus:border-gray-400"
            placeholder="0.1"
            required
          />
        </div>

        {/* Submit */}
        <button
          type="submit"
          disabled={loading}
          className={`w-full py-3 rounded font-semibold text-sm transition-opacity ${
            loading ? 'opacity-50 cursor-not-allowed' : 'opacity-100'
          } ${
            side === 'buy'
              ? 'bg-[#26a69a] text-black'
              : 'bg-[#ef5350] text-black'
          }`}
        >
          {loading
            ? 'Submitting...'
            : `${side === 'buy' ? 'Buy' : 'Sell'} ${type === 'market' ? 'Market' : 'Limit'}`}
        </button>
      </form>

      {/* Status */}
      {status && (
        <div
          className={`mt-3 p-2 rounded text-xs font-mono ${
            status.startsWith('Error') || status.startsWith('Network')
              ? 'bg-red-900/30 text-red-400 border border-red-800'
              : 'bg-green-900/30 text-green-400 border border-green-800'
          }`}
        >
          {status}
        </div>
      )}
    </div>
  );
}
