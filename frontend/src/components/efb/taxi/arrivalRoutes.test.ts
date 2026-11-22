import { describe, expect, it } from 'vitest';
import { defaultArrivalClearance } from './arrivalRoutes';
import { computeArrivalRoute } from './taxiRoute';

describe('defaultArrivalClearance', () => {
  it.each([
    ['22L', 'F98', 'B.30'], ['22L', 'G120', 'B.30'], ['22L', 'A25', 'B.30'],
    ['22L', 'A4', 'B.C'], ['22L', 'A17', 'B.C'], ['22L', 'a23', 'B.C'],
    ['04L', 'F98', 'A.30'], ['04L', 'A4', 'A.30'], ['RWY04L', '', 'A.30'],
  ])('%s to %s -> %s', (runway, stand, clearance) => {
    expect(defaultArrivalClearance(runway, stand)).toBe(clearance);
  });

  it.each(['22R', '04R', '12', '30', '', null])('has no standard routing for runway %s', (runway) => {
    expect(defaultArrivalClearance(runway, 'F98')).toBeNull();
  });

  it.each([['22L', 'F98', '22L · B', '30'], ['22L', 'A4', '22L · B', 'C'], ['04L', 'F98', '04L · A', '30']])(
    'draws the standard routing from %s for %s', (runway, stand, startLabel, hold) => {
      const route = computeArrivalRoute(runway, stand, defaultArrivalClearance(runway, stand)!);
      expect(route.startLabel).toBe(startLabel);
      expect(route.marks.map((m) => `${m.type}:${m.label}`)).toEqual([`hold:${hold}`]);
      expect(route.messages.filter((m) => m.level === 'err')).toEqual([]);
    },
  );
});
