import type { Metadata } from 'next';
import './globals.css';
import { Providers } from './providers';

export const metadata: Metadata = {
  title: 'Perp Crisis Sandbox',
  description: 'Perpetuals trading simulator — disaster scenario replay',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body
        className="antialiased"
        style={{
          backgroundColor: '#0f0f0f',
          color: '#d1d4dc',
          margin: 0,
          padding: 0,
          overflow: 'hidden',
        }}
      >
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
