'use client';

// Wagmi + RainbowKit + TanStack Query providers.
//
// Notes:
//   - Wagmi v2 requires a TanStack Query Client (we create one per browser
//     tab; React 18's strict-mode double-invoke is fine because we memoise).
//   - WalletConnect needs a projectId. For local dev we read it from
//     NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID; if absent we use a tag value so
//     the build doesn't fail, but cross-device QR won't work without a real
//     id (get one at https://cloud.walletconnect.com — free for hobby use).

import '@rainbow-me/rainbowkit/styles.css';
import { RainbowKitProvider, getDefaultConfig, darkTheme } from '@rainbow-me/rainbowkit';
import { WagmiProvider } from 'wagmi';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState } from 'react';
import { SUPPORTED_CHAINS } from '@/lib/chains';

const WC_PROJECT_ID = process.env.NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID || 'perpcs-dev-placeholder';

const wagmiConfig = getDefaultConfig({
  appName: 'Perp Crisis Sandbox',
  projectId: WC_PROJECT_ID,
  chains: SUPPORTED_CHAINS,
  ssr: true,
});

export function Providers({ children }: { children: React.ReactNode }) {
  // Memoise the QueryClient across renders inside this provider tree.
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // Wallet-related queries don't need to be retried aggressively;
            // most of our reads are wallet-local or cheap RPC views.
            retry: 1,
            staleTime: 5_000,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <WagmiProvider config={wagmiConfig}>
      <QueryClientProvider client={queryClient}>
        <RainbowKitProvider
          theme={darkTheme({ accentColor: '#26a69a', borderRadius: 'small' })}
          modalSize="compact"
        >
          {children}
        </RainbowKitProvider>
      </QueryClientProvider>
    </WagmiProvider>
  );
}
