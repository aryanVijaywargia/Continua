import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Continua - Debug UI',
  description: 'Debug and inspect agent executions',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="bg-gray-900 text-gray-100">{children}</body>
    </html>
  );
}
