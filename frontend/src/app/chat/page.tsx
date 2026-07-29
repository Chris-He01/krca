'use client';

import { Suspense } from 'react';
import { AgentWorkspace } from '@/components/AgentWorkspace';

export default function ChatPage() {
  return (
    <main className="min-h-screen">
      <Suspense>
        <AgentWorkspace />
      </Suspense>
    </main>
  );
}
