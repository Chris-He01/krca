import type { Metadata } from 'next';
import { Inter } from 'next/font/google';
import './globals.css';
import { LanguageProvider } from '@/contexts/LanguageContext';
import { UserProvider } from '@/contexts/UserContext';

const inter = Inter({ subsets: ['latin'] });

export const metadata: Metadata = {
  title: 'Knsight - AI-Powered Root Cause Analysis',
  description: 'An explainable RCA system powered by LLM and Multi-Agent architecture, reducing diagnosis time from hours to minutes',
  icons: {
    icon: '/favicon.png',
    apple: '/favicon.png',
  },
  // Discourage Chrome's auto-translate / LanguageDetector AI from rewriting the
  // DOM — that text mutation otherwise produces hydration mismatches (React #418).
  other: {
    google: 'notranslate',
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="zh-CN" translate="no" suppressHydrationWarning>
      <body className={`${inter.className} notranslate`} suppressHydrationWarning>
        <UserProvider>
          <LanguageProvider>{children}</LanguageProvider>
        </UserProvider>
      </body>
    </html>
  );
}
