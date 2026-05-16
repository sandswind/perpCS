'use client';

// v0.5 Landing page — Connect Wallet → Faucet → Level Select → Deposit.
//
// SIWE login is a soft step here: the v0.5 backend doesn't yet require an
// authenticated session for /sessions/{addr} (the on-chain SessionStarted
// event is the proof). We expose a "Sign in with Ethereum" button anyway so
// the FE plumbing is in place for v0.6's signed withdrawal path.

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { ConnectButton } from '@rainbow-me/rainbowkit';
import { useAccount, useChainId, useSwitchChain } from 'wagmi';
import FaucetCard from '@/components/onchain/FaucetCard';
import DepositCard from '@/components/onchain/DepositCard';
import { DEFAULT_CHAIN, DEFAULT_CHAIN_ID } from '@/lib/chains';
import { loadDeployment, type Deployment } from '@/lib/deployments';

export default function LandingPage() {
  const { isConnected } = useAccount();
  const chainId = useChainId();
  const { switchChain } = useSwitchChain();
  const [dep, setDep] = useState<Deployment | null>(null);
  const [depErr, setDepErr] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    loadDeployment(DEFAULT_CHAIN_ID)
      .then((d) => {
        if (!cancelled) setDep(d);
      })
      .catch((e) => {
        if (!cancelled) setDepErr(String(e?.message || e));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const wrongChain = isConnected && chainId !== DEFAULT_CHAIN_ID;

  return (
    <div
      className="min-h-screen w-screen overflow-y-auto"
      style={{ backgroundColor: '#0f0f0f', color: '#d1d4dc' }}
    >
      <header className="flex items-center justify-between px-6 py-4 border-b border-gray-800">
        <div>
          <h1 className="text-lg font-bold">Perp Crisis Sandbox</h1>
          <p className="text-xs text-gray-500">
            v0.5 · {DEFAULT_CHAIN.name} (chainId {DEFAULT_CHAIN.id})
          </p>
        </div>
        <ConnectButton chainStatus="icon" showBalance={false} accountStatus="address" />
      </header>

      <main className="max-w-3xl mx-auto px-6 py-8 space-y-6">
        {/* Wrong-chain banner */}
        {wrongChain && (
          <div className="rounded border border-yellow-700 bg-yellow-900/20 p-4 text-sm">
            <div className="font-semibold text-yellow-300 mb-1">Wrong network</div>
            <p className="text-yellow-200/80 mb-2">
              The sandbox is deployed on {DEFAULT_CHAIN.name} (chainId {DEFAULT_CHAIN.id}).
              Switch your wallet to continue.
            </p>
            <button
              onClick={() => switchChain({ chainId: DEFAULT_CHAIN_ID })}
              className="px-3 py-1.5 rounded bg-yellow-500 text-black text-xs font-semibold hover:bg-yellow-400"
            >
              Switch to {DEFAULT_CHAIN.name}
            </button>
          </div>
        )}

        {/* Deployment-not-set banner */}
        {depErr && (
          <div className="rounded border border-red-700 bg-red-900/20 p-4 text-sm">
            <div className="font-semibold text-red-300 mb-1">Contracts not deployed yet</div>
            <p className="text-red-200/80 font-mono whitespace-pre-wrap break-all">{depErr}</p>
            <p className="text-red-200/60 text-xs mt-2">
              Run <code>cd contracts &amp;&amp; forge script script/Deploy.s.sol --broadcast</code>{' '}
              and refresh this page. The deploy script writes{' '}
              <code>deployments/&lt;chain&gt;.json</code>; copy it to{' '}
              <code>web/public/deployments/</code> (handled by the Makefile).
            </p>
          </div>
        )}

        {!isConnected && (
          <div className="rounded border border-gray-800 bg-[#111] p-6 text-center">
            <h2 className="text-base font-semibold mb-2">Step 0 — Connect your wallet</h2>
            <p className="text-sm text-gray-400 mb-4">
              MetaMask, Rainbow, or any WalletConnect-compatible wallet works. We never
              touch real funds — all gas + tokens are testnet only.
            </p>
            <div className="inline-block">
              <ConnectButton />
            </div>
          </div>
        )}

        {isConnected && !wrongChain && dep && <FaucetCard deployment={dep} />}
        {isConnected && !wrongChain && dep && <DepositCard deployment={dep} />}

        <div className="text-center pt-4 border-t border-gray-800/40">
          <Link href="/trade" className="text-xs text-gray-600 hover:text-gray-400">
            ← Skip to v0.4 demo (no wallet)
          </Link>
        </div>
      </main>
    </div>
  );
}
