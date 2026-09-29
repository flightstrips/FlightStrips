import { describe, expect, it } from 'vitest';
import { idempotencyHeaders } from './http-idempotency';

describe('HTTP idempotency headers', () => {
  it('adds a canonical UUID only to mutations', () => {
    expect(idempotencyHeaders({ method: 'GET' })).toEqual({});
    const first = idempotencyHeaders({ method: 'POST' })['Idempotency-Key'];
    const second = idempotencyHeaders({ method: 'PUT' })['Idempotency-Key'];
    expect(first).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/);
    expect(second).not.toBe(first);
    expect(idempotencyHeaders({ method: 'POST', headers: { 'Idempotency-Key': first } })).toEqual({});
  });
});
