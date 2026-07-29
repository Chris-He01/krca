'use client';

import dynamic from 'next/dynamic';
import { useEffect, useState } from 'react';

// Disable SSR for the dashboard to avoid hydration mismatches caused by:
// - Date/locale formatting differing between build-time and client
// - Browser extensions (Chrome's LanguageDetector AI) mutating DOM
// - Static export pre-rendering with stale build-time state
const DashboardClient = dynamic(() => import('./DashboardClient'), {
  ssr: false,
  loading: () => (
    <div style={{ height: '100vh', display: 'grid', placeItems: 'center', background: 'hsl(var(--background))' }}>
      <div style={{ textAlign: 'center' }}>
        <div style={{ width: 6, height: 6, borderRadius: 99, background: '#f59e0b', margin: '0 auto 12px' }} />
        <div style={{ fontFamily: 'ui-monospace, "SF Mono", Menlo, monospace', fontSize: 11, color: 'hsl(var(--muted-foreground) / 0.6)', letterSpacing: '0.06em', textTransform: 'uppercase' }}>Loading dashboard...</div>
      </div>
    </div>
  ),
});

export default function DiagnosticsPage() {
  // Render-only-after-mount guard to bypass any SSG snapshot
  const [mounted, setMounted] = useState(false);
  useEffect(() => { setMounted(true); }, []);
  if (!mounted) {
    return (
      <div suppressHydrationWarning style={{ height: '100vh', display: 'grid', placeItems: 'center', background: 'hsl(var(--background))' }}>
        <div style={{ textAlign: 'center' }}>
          <div style={{ width: 6, height: 6, borderRadius: 99, background: '#f59e0b', margin: '0 auto 12px' }} />
          <div style={{ fontFamily: 'ui-monospace, "SF Mono", Menlo, monospace', fontSize: 11, color: 'hsl(var(--muted-foreground) / 0.6)', letterSpacing: '0.06em', textTransform: 'uppercase' }}>Loading dashboard...</div>
        </div>
      </div>
    );
  }
  return <DashboardClient />;
}
