import { API_BASE } from './config';

export async function placeOrder(
  side: 'buy' | 'sell',
  type: 'market' | 'limit',
  qty: string,
  price?: string
): Promise<unknown> {
  const res = await fetch(`${API_BASE}/orders`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      side,
      type,
      quantity: qty,
      price: price || '0',
    }),
  });
  return res.json();
}

export async function cancelOrder(id: number): Promise<unknown> {
  const res = await fetch(`${API_BASE}/orders/${id}`, { method: 'DELETE' });
  return res.json();
}

export async function getAccount(): Promise<unknown> {
  const res = await fetch(`${API_BASE}/account`);
  return res.json();
}
