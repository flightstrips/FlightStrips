// The caller owns retries and must retain RequestInit for an uncertain action.
export function idempotencyHeaders(init?: RequestInit): Record<string, string> {
  const method = (init?.method ?? 'GET').toUpperCase();
  if (!['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) return {};
  if (new Headers(init?.headers).has('Idempotency-Key')) return {};
  return { 'Idempotency-Key': crypto.randomUUID() };
}
