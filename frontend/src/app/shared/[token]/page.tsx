import { SharedSessionClient } from './SharedSessionClient';

// Static export requires at least one pre-rendered path.
// Tokens are runtime-only, so we use a placeholder — actual tokens
// are resolved at runtime via Go backend SPA fallback → client router.
export function generateStaticParams() {
  return [{ token: '__placeholder__' }];
}

export default function SharedSessionPage() {
  return <SharedSessionClient />;
}
