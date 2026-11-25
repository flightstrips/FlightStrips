import { describe, expect, it } from 'vitest';
import { EKCH_COMMON_ROUTES, commonRoutesFor, type CommonRoute } from './commonRoutes';
import { computeTaxiRoute, parseClearance } from './taxiRoute';

const routes: CommonRoute[] = [
  { runway: '22R', holdingPoint: 'A4', clearance: 'KZAx30A4.22R', releasePoints: ['Z3', 'Z4'] },
  { runway: '22R', holdingPoint: 'A1', clearance: 'KZAx30.22R' },
  { runway: '04L', holdingPoint: 'A4', clearance: 'KZAx30A4.04L' },
];

describe('commonRoutesFor', () => {
  it('offers routes for the runway in use and the release point', () => {
    expect(commonRoutesFor('22r', 'z3', routes).map((r) => r.holdingPoint)).toEqual(['A4', 'A1']);
    expect(commonRoutesFor('22R', 'J1', routes).map((r) => r.holdingPoint)).toEqual(['A1']);
    expect(commonRoutesFor('22R', '', routes)).toHaveLength(2);
    expect(commonRoutesFor('30', 'Z3', routes)).toEqual([]);
  });
});

describe('EKCH common routes', () => {
  it('are all readable, and draw without errors from the release points they name', () => {
    for (const route of EKCH_COMMON_ROUTES) {
      const parsed = parseClearance(route.clearance);
      expect(parsed.overLimit ?? false, route.clearance).toBe(false);
      expect(parsed.bad, route.clearance).toBe('');
      for (const rp of route.releasePoints ?? []) {
        const errors = computeTaxiRoute(rp, route.clearance).messages.filter((m) => m.level === 'err');
        expect(errors, `${route.clearance} from ${rp}`).toEqual([]);
      }
    }
  });
});
