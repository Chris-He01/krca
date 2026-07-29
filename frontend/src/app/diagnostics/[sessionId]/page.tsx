import DiagnosticDetailClient from './DiagnosticDetailClient';

export function generateStaticParams() {
  return [{ sessionId: '__placeholder__' }];
}

export default function DiagnosticDetailPage() {
  return <DiagnosticDetailClient />;
}
