import { describe, expect, it } from 'vitest';
import { standReleasePoint } from './standReleasePoints';
import { findReleasePoint } from './taxiRoute';

describe('standReleasePoint', () => {
  it.each([
    ['G110', 'I'], ['G112', 'I'], ['g115', 'I'],
    ['G117', 'G6'], ['G120', 'G6'], ['G124', 'G6'],
    ['G125', 'G'], ['G137', 'G'],
    ['G15', 'G'], ['G19', 'G'],
  ])('%s -> %s', (stand, point) => {
    expect(standReleasePoint(stand)).toBe(point);
  });

  it.each(['G109', 'G116', 'G138', 'G14', 'G20', 'A12', 'F110', '', null])('%s has no fixed release point', (stand) => {
    expect(standReleasePoint(stand)).toBeNull();
  });

  it('only uses release points that are on the taxi map', () => {
    for (const point of ['I', 'G6', 'G']) expect(findReleasePoint(point)?.name).toBe(point);
  });
});
