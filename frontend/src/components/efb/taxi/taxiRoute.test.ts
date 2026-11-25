import { describe, expect, it } from 'vitest';
import { arcs, computeArrivalRoute, computeTaxiRoute, EKCH_TAXI_NETWORK, holdBack, findReleasePoint, findStand, impliedDepartureRunway, MAX_CLEARANCE_LENGTH, parseClearance, routePoints, type TaxiRoute } from './taxiRoute';

const summary = (raw: string) => {
  const parsed = parseClearance(raw);
  return [...parsed.items.map((item) => `${item.type}:${item.end ?? item.name}`), ...(parsed.bad ? [`bad:${parsed.bad}`] : [])].join(' ');
};
const legNames = (route: TaxiRoute) => [...route.legs.map((leg) => leg.name), ...(route.stub ? [`${route.stub.name}>`] : [])];
const marks = (route: TaxiRoute) => route.marks.map((mark) => `${mark.type}:${mark.label}${mark.implicit ? '!' : ''}`);
const levels = (route: TaxiRoute, level: string) => route.messages.filter((m) => m.level === level).map((m) => m.text);

describe('parseClearance', () => {
  it.each([
    ['KZAA4', 'twy:K twy:Z twy:A twy:A4'],
    ['k z a a4', 'twy:K twy:Z twy:A twy:A4'],
    ['KZ.A', 'twy:K twy:Z hold:A'],
    ['TV.22L', 'twy:T twy:V hold:22L'],
    ['KZAx30A4.22R', 'twy:K twy:Z twy:A cross:30 twy:A4 hold:22R'],
    ['K Z A cross 30 A4, 22R', 'twy:K twy:Z twy:A cross:30 twy:A4 hold:22R'],
    ['GG1x30', 'twy:G twy:G1 cross:30'],
    ['FF1F2', 'twy:F twy:F1 twy:F2'],
    ['XQ', 'twy:X twy:Q'],
    ['X.22L', 'twy:X hold:22L'],
    ['KZA30', 'twy:K twy:Z twy:A rwy:30'],
    ['kzaa4.rwy22r', 'twy:K twy:Z twy:A twy:A4 hold:22R'],
    ['KZQQ', 'twy:K twy:Z twy:Q twy:Q'],
    ['KZ?', 'twy:K twy:Z bad:?'],
  ])('%s', (raw, expected) => {
    expect(summary(raw)).toBe(expected);
  });

  it('maps both runway ends to the same runway', () => {
    expect(parseClearance('.04L').items[0].name).toBe('04L/22R');
    expect(parseClearance('.22R').items[0].name).toBe('04L/22R');
  });

  it('rejects an oversized clearance before tokenizing it', () => {
    expect(parseClearance('A'.repeat(MAX_CLEARANCE_LENGTH)).items).toHaveLength(MAX_CLEARANCE_LENGTH);
    const route = computeTaxiRoute('Z3', 'A'.repeat(20_000));
    expect(route.legs).toEqual([]);
    expect(levels(route, 'err')).toEqual([`Limit the clearance to ${MAX_CLEARANCE_LENGTH} characters.`]);
  });
});

describe('computeTaxiRoute', () => {
  it('starts at the release point and leaves the first taxiway open-ended', () => {
    const route = computeTaxiRoute('Z3', 'K');
    expect(route.start).not.toBeNull();
    expect(legNames(route)).toEqual(['K>']);
  });

  it('marks the mapped A5 holding point without joining the runway', () => {
    const route = computeTaxiRoute('Z3', 'KZAx30A5.22R');
    expect(legNames(route)).toEqual(['K', 'Z', 'A', 'A5']);
    const hold = route.marks.find((mark) => mark.type === 'hold');
    expect(hold).toMatchObject({ label: '22R', atHoldPoint: true });
    expect(route.legs.at(-1)?.nodes.at(-1)?.n).toBe('E35a');
    expect(levels(route, 'warn')).toEqual([]);
    expect(levels(route, 'err')).toEqual([]);
  });

  it('checks runway crossings reached by an open-ended taxiway', () => {
    const uncleared = computeTaxiRoute('Y3', 'YAF2');
    expect(uncleared.stub?.name).toBe('F2');
    expect(marks(uncleared)).toEqual(['hold:12/30!']);
    expect(levels(uncleared, 'err')[0]).toContain('No crossing clearance for 12/30');
    expect(uncleared.holdAt).not.toBeNull();

    const cleared = computeTaxiRoute('Y3', 'YAF2x30');
    expect(marks(cleared)).toEqual(['cross:30']);
    expect(levels(cleared, 'err')).toEqual([]);
  });

  it('keeps a hold short of A anchored at the Y/A junction', () => {
    const route = computeTaxiRoute('P2', 'PYA,A.30');
    const junctions = EKCH_TAXI_NETWORK.nodes.filter(({ names }) => names.includes('Y') && names.includes('A'));
    const hold = route.marks.find((mark) => mark.type === 'hold' && mark.name === 'A');
    expect(hold).toBeDefined();
    const [x, y] = routePoints(route)[hold!.g];
    expect(Math.min(...junctions.map((node) => Math.hypot(x - node.x, y - node.y)))).toBeLessThan(5);
  });

  it('draws earlier taxiways in full and keeps the arrow on the newest one', () => {
    expect(legNames(computeTaxiRoute('Z3', 'KZ'))).toEqual(['K', 'Z>']);
  });

  it('draws up to a hold short and marks it', () => {
    const route = computeTaxiRoute('Z3', 'KZAx30A4.22R');
    expect(legNames(route)).toEqual(['K', 'Z', 'A', 'A4']);
    expect(marks(route)).toEqual(['cross:30', 'hold:22R']);
    expect(levels(route, 'err')).toEqual([]);
  });

  it('holds short of a runway crossed without a crossing clearance', () => {
    const route = computeTaxiRoute('Z3', 'KZAA4.22R');
    expect(marks(route)).toEqual(['hold:12/30!', 'hold:22R']);
    expect(levels(route, 'err')[0]).toContain('No crossing clearance for 12/30');
    expect(route.holdAt).not.toBeNull();
  });

  it('reports taxiways that do not connect', () => {
    expect(levels(computeTaxiRoute('Z3', 'KB'), 'err')).toEqual(['K does not connect to B']);
  });

  it('taxis along a runway in the written direction only', () => {
    expect(legNames(computeTaxiRoute('Z3', 'KZA30K2'))).toEqual(['K', 'Z', 'A', '30', 'K2>']);
    expect(levels(computeTaxiRoute('Z3', 'KZA12K2'), 'err')).toEqual(['Heading 12 on the runway does not reach K2']);
    expect(levels(computeTaxiRoute('Z3', 'KZA12/30'), 'err')[0]).toContain('Say which direction');
  });

  it('joins 22R via A1 and 22L via V1 when the last taxiway does not reach the runway', () => {
    const r22 = computeTaxiRoute('Z3', 'KZAx30.22R');
    expect(legNames(r22)).toEqual(['K', 'Z', 'A', 'A1']);
    expect(marks(r22)).toEqual(['cross:30', 'hold:22R']);
    expect(legNames(computeTaxiRoute('T3', 'TV.22L'))).toEqual(['T', 'V', 'V1']);
    expect(levels(computeTaxiRoute('Z3', 'KZx30.22R'), 'warn')).toContain('Hold short 22R: Z does not lead to A1');
  });

  it('routes R to Y via V', () => {
    const route = computeTaxiRoute('R1', 'RY');
    expect(legNames(route)).toEqual(['R', 'V', 'Y>']);
    expect(levels(route, 'info')).toEqual(['R to Y via V (standard routing)']);
  });

  it('smooths P-Y-A into one curve without moving the marks after it', () => {
    const route = computeTaxiRoute('P2', 'PYAx30A4.22R');
    expect(legNames(route)).toEqual(['P', 'Y', 'A', 'A4']);
    expect(route.legs[1].pts.length).toBe(25);
    expect(marks(route)).toEqual(['cross:30', 'hold:22R']);
  });

  it.each(['B1', 'B2', 'B3'])('connects B to %s at the map edge to hold short of 04R', (taxiway) => {
    const route = computeTaxiRoute('B', `Bx30${taxiway}.04R`);
    expect(legNames(route)).toEqual(['B', taxiway]);
    expect(marks(route)).toEqual(['cross:30', 'hold:04R']);
    expect(route.legs[1].offMap).toBe(true);
    expect(levels(route, 'err')).toEqual([]);
    expect(levels(route, 'info')).toContain(`${taxiway} is beyond the edge of the map; the route is drawn to where it joins.`);
    // Nothing is drawn past the map edge: the off-map leg has no length.
    const [a, b] = [route.legs[1].pts[0], route.legs[1].pts.at(-1)!];
    expect(Math.hypot(a[0] - b[0], a[1] - b[1])).toBe(0);
    expect(a[1]).toBeLessThanOrEqual(2286.5);
  });

  it('does not offer B1-B3 as release points', () => {
    for (const name of ['B1', 'B2', 'B3', 'E2', 'E3', 'E4']) expect(findReleasePoint(name)).toBeNull();
  });

  it('rejects unknown release points', () => {
    expect(findReleasePoint('z3')?.name).toBe('Z3');
    expect(levels(computeTaxiRoute('Q9', 'K'), 'err')).toEqual(['Unknown release point "Q9"']);
  });
});

describe('computeArrivalRoute', () => {
  const arrival = (runway: string, stand: string, clearance: string) => computeArrivalRoute(runway, stand, clearance);

  it('gives the hold instruction even when the route geometry reaches the stand', () => {
    const route = arrival('22L', 'F98', 'V1V,V');
    expect(route.standReached).toBe(true);
    expect(marks(route)).toContain('hold:V');
    expect(levels(route, 'ok')).toEqual(['Hold short of taxiway V.']);
  });

  it('starts where the first taxiway leaves the landing runway and ends at the stand', () => {
    const route = arrival('22L', 'F98', 'V1V');
    expect(route.mode).toBe('arrival');
    expect(route.startLabel).toBe('22L · V1');
    expect(legNames(route)).toEqual(['V1', 'V']);
    expect(route.standReached).toBe(true);
    expect(route.stub).toBeNull();
    expect(levels(route, 'ok')).toEqual(['Route OK. Taxi to stand F98.']);
    // The route ends where the stand joins V.
    const end = route.legs[1].pts.at(-1)!;
    expect(Math.hypot(end[0] - route.standEntry![0], end[1] - route.standEntry![1])).toBeLessThan(1);
  });

  it('leaves the last taxiway open-ended until the stand taxiway is in the clearance', () => {
    const route = arrival('22L', 'F98', 'V1');
    expect(legNames(route)).toEqual(['V1>']);
    expect(route.standReached).toBe(false);
    expect(levels(route, 'info')).toContain('Stand F98 is on V; the route reaches it once V is in the clearance.');
  });

  it('applies the same hold short and crossing rules as departures', () => {
    const route = arrival('22R', 'A12', 'A4AZ');
    expect(marks(route)).toEqual(['hold:12/30!']);
  });

  it('reports exits that do not leave the landing runway, unknown runways and stands', () => {
    expect(levels(arrival('22L', 'F98', 'K'), 'err')).toEqual(['K does not leave runway 22L']);
    expect(levels(arrival('27', 'F98', 'V1'), 'err')).toEqual(['Unknown runway "27"']);
    expect(levels(arrival('', 'F98', 'V1'), 'warn')).toEqual(['Enter the landing runway']);
    expect(levels(arrival('22L', 'XX1', 'V1V'), 'warn')).toContain('Stand XX1 is not on the taxi map');
  });

  it('knows where every stand is', () => {
    for (const stand of ['A4', 'B19', 'C39', 'D4', 'E90', 'F98', 'G137', 'H106', 'RIII', 'W1']) expect(findStand(stand)?.name).toBe(stand);
  });
});

describe('onward clearances with "/"', () => {
  it.each(['KZA.30/', 'KZA.30// '])('keeps the hold active while %s has no onward instruction', (clearance) => {
    const route = computeTaxiRoute('Z3', clearance);
    expect(route.parsed.activeSegment).toBe(0);
    expect(route.legs.some((leg) => leg.past)).toBe(false);
    expect(marks(route)).toEqual(['hold:30']);
    expect(levels(route, 'err')).toEqual([]);
  });

  it('splits the clearance into parts without breaking runway names', () => {
    const parsed = parseClearance('KZ.A/Ax12/30A4.22R');
    expect(parsed.activeSegment).toBe(1);
    expect(parsed.items.map((i) => `${i.type}:${i.end ?? i.name}@${i.segment}`)).toEqual([
      'twy:K@0', 'twy:Z@0', 'hold:A@0', 'twy:A@1', 'cross:12/30@1', 'twy:A4@1', 'hold:22R@1',
    ]);
    expect(parsed.bad).toBe('');
  });

  it('continues the route, greys out what came before the last "/", and releases its hold shorts', () => {
    const route = computeTaxiRoute('Z3', 'KZ.A/Ax30A4.22R');
    expect(legNames(route)).toEqual(['K', 'Z', 'A', 'A4']);
    expect(route.legs.map((leg) => Boolean(leg.past))).toEqual([true, true, false, false]);
    expect(marks(route)).toEqual(['cross:30', 'hold:22R']);
    expect(levels(route, 'err')).toEqual([]);
  });

  it('continues the onward clearance from the earlier hold short, also on the same taxiway', () => {
    const route = computeTaxiRoute('B', 'Bx30.Z/BVT');
    expect(legNames(route)).toEqual(['B', 'B', 'V', 'T>']);
    expect(route.legs.map((leg) => Boolean(leg.past))).toEqual([true, false, false]);
    // The first B runs to the Z junction; the onward B starts exactly there.
    const [done, onward] = route.legs;
    expect(onward.pts[0]).toEqual(done.pts.at(-1));
    expect(levels(route, 'err')).toEqual([]);
  });

  it('turns back from the runway hold on B and holds short of Z', () => {
    const route = computeTaxiRoute('B', 'B.30/.Z');
    expect(legNames(route)).toEqual(['B', 'B']);
    expect(route.legs.map((leg) => leg.past)).toEqual([true, false]);
    expect(marks(route)).toEqual(['hold:Z']);
    expect(levels(route, 'err')).toEqual([]);
    expect(levels(route, 'warn')).toEqual([]);
    expect(levels(route, 'ok')).toEqual(['Route OK.']);
  });

  it.each(['KZAx30A4/AZK', 'KZAx30A4AZK'])('requires another crossing clearance for %s', (clearance) => {
    const route = computeTaxiRoute('Z3', clearance);
    expect(marks(route)).toEqual(['cross:30', 'hold:12/30!']);
    expect(levels(route, 'ok')).toEqual([]);
    expect(route.holdAt).not.toBeNull();
  });

  it('allows repeated crossings when each occurrence is cleared', () => {
    const route = computeTaxiRoute('Z3', 'KZAx30A4/Ax12ZK');
    expect(marks(route)).toEqual(['cross:30', 'cross:12']);
    expect(levels(route, 'err')).toEqual([]);
  });

  it('does not carry an unused crossing to a later clearance', () => {
    const route = computeTaxiRoute('Z3', 'KZx30.A/AA4.22R');
    expect(marks(route)).toEqual(['hold:12/30!', 'hold:22R']);
    expect(levels(route, 'warn')).toContain('Cross 30: the route does not cross it');
  });

  it.each(['KZA.30/Ax30A4.22R', 'KZA.30/x30A4.22R'])('uses the onward crossing clearance in %s', (clearance) => {
    const route = computeTaxiRoute('Z3', clearance);
    expect(marks(route)).toEqual(['cross:30', 'hold:22R']);
    expect(levels(route, 'err')).toEqual([]);
    expect(levels(route, 'warn')).toEqual([]);
    expect(route.legs.filter((leg) => !leg.past).map((leg) => leg.name)).toEqual(['A', 'A4']);
  });

  it.each([
    ['KZ/.A', ['hold:A']],
    ['KZA/.A', ['hold:A']],
    ['KZA/.30', ['hold:30']],
    ['KZAx30/.A4', ['cross:30', 'hold:A4']],
    ['KZAx30/.22R', ['cross:30', 'hold:22R']],
    ['KZA.30/x30.22R', ['cross:30', 'hold:22R']],
  ])('draws the hold in the onward clearance %s', (clearance, expected) => {
    const route = computeTaxiRoute('Z3', clearance);
    expect(marks(route)).toEqual(expected);
    expect(route.holdAt).not.toBeNull();
    expect(levels(route, 'err')).toEqual([]);
    expect(levels(route, 'warn')).toEqual([]);
  });

  it('keeps an open-ended arrow on the active part', () => {
    const route = computeTaxiRoute('Z3', 'K.Z/Z');
    expect(legNames(route)).toEqual(['K', 'Z>']);
    expect(route.legs[0].past).toBe(true);
    expect(route.stub?.past).toBe(false);
  });
});

describe('stands reachable from more than one taxiway', () => {
  it.each([
    ['C27', ['R', 'S']], ['D1', ['R', 'S']], ['D2', ['R', 'S']], ['D3', ['S', 'R']], ['D4', ['S', 'R']],
    ['B10', ['M', 'Y']], ['A21', ['J', 'L']], ['A30', ['J', 'K']],
  ])('%s joins %j', (stand, taxiways) => {
    expect(findStand(stand)!.entries.map((entry) => entry.taxiway)).toEqual(taxiways);
  });

  it.each([
    ['C27', 'V1VR'], ['C27', 'V1VS'], ['D1', 'V1VS'], ['D3', 'V1VR'],
    ['B10', 'V1VY'], ['B10', 'V1VYM'], ['A21', 'V1VYL'], ['A30', 'V1VYLK'],
  ])('reaches %s via %s', (stand, clearance) => {
    const route = computeArrivalRoute('22L', stand, clearance);
    expect(route.messages.filter((m) => m.level === 'err')).toEqual([]);
    expect(route.standReached).toBe(true);
    const end = route.legs.at(-1)!.pts.at(-1)!;
    expect(Math.hypot(end[0] - route.standEntry![0], end[1] - route.standEntry![1])).toBeLessThan(1);
  });

  it('names every taxiway a stand joins when it is not reached yet', () => {
    expect(computeArrivalRoute('22L', 'C27', 'V1V').messages.map((m) => m.text)).toContain('Stand C27 is on R or S; the route reaches it once R or S is in the clearance.');
  });
});

describe('holding point written first', () => {
  it.each([
    ['Z3', 'A4KZAx30', ['K', 'Z', 'A', 'A4'], ['cross:30', 'hold:22R']],
    ['Z3', 'A4 K Z A x30', ['K', 'Z', 'A', 'A4'], ['cross:30', 'hold:22R']],
    ['Z3', 'A1KZAx30', ['K', 'Z', 'A', 'A1'], ['cross:30', 'hold:22R']],
    ['B', 'B1Bx30', ['B', 'B1'], ['cross:30', 'hold:04R']],
    ['B', 'B4Bx30', ['B', 'B4'], ['cross:30', 'hold:04R']],
    ['Z3', 'K1K', ['K', 'K1'], ['hold:12']],
  ])('%s %s taxis to the holding point last', (rp, clearance, legs, holds) => {
    const route = computeTaxiRoute(rp, clearance);
    expect(legNames(route)).toEqual(legs);
    expect(marks(route)).toEqual(holds);
    expect(levels(route, 'err')).toEqual([]);
    expect(levels(route, 'info')[0]).toMatch(/^Holding point \w+ written first/);
  });

  it.each(['A122RKZAx30', '.A122RKZAx30', 'A1 22R K Z A x30', 'A1.22RKZAx30'])('moves a holding point written with its runway (%s)', (clearance) => {
    const route = computeTaxiRoute('Z3', clearance);
    expect(legNames(route)).toEqual(['K', 'Z', 'A', 'A1']);
    expect(marks(route)).toEqual(['cross:30', 'hold:22R']);
    expect(levels(route, 'err')).toEqual([]);
  });

  it('uses the runway end as written', () => {
    expect(marks(computeTaxiRoute('Z3', 'A104LKZAx30'))).toEqual(['cross:30', 'hold:04L']);
  });

  it('leaves a hold short of a holding point without its runway alone', () => {
    expect(computeTaxiRoute('Z3', '.A1KZA').parsed.items[0]).toMatchObject({ type: 'hold', name: 'A1' });
  });

  it('keeps an explicit hold short instead of adding one', () => {
    expect(marks(computeTaxiRoute('Z3', 'A4KZAx30.22R'))).toEqual(['cross:30', 'hold:22R']);
  });

  it('does not add a duplicate departure runway hold in the onward clearance', () => {
    const route = computeTaxiRoute('Z3', 'A4KZA.22R/A');
    expect(route.parsed.items.filter((item) => item.type === 'hold')).toEqual([
      expect.objectContaining({ end: '22R', segment: 0 }),
    ]);
    // The holding point ends the clearance it was written in, ahead of that clearance's hold short.
    expect(route.parsed.items.map((item) => `${item.type}:${item.name}@${item.segment}`)).toEqual([
      'twy:K@0', 'twy:Z@0', 'twy:A@0', 'twy:A4@0', 'hold:04L/22R@0', 'twy:A@1',
    ]);
  });

  it('reaches a leading holding point within its own clearance before an onward one', () => {
    const route = computeTaxiRoute('Z3', 'A4KZAx30.22R/A', { departureRunway: '22R' });
    expect(route.parsed.items.map((item) => `${item.type}:${item.name}@${item.segment}`)).toEqual([
      'twy:K@0', 'twy:Z@0', 'twy:A@0', 'cross:12/30@0', 'twy:A4@0', 'hold:04L/22R@0', 'twy:A@1',
    ]);
    expect(levels(route, 'err')).toEqual([]);
    expect(route.legs.filter((leg) => leg.past).map((leg) => leg.name)).toEqual(['K', 'Z', 'A', 'A4']);
  });

  it('rejects a leading holding point for the other end of the assigned runway', () => {
    const route = computeTaxiRoute('Z3', 'A4KZAx30', { departureRunway: '04L' });
    expect(route.legs).toEqual([]);
    expect(levels(route, 'err')).toEqual(['Holding point A4 is for runway 22R, which conflicts with assigned runway 04L.']);
    expect(levels(computeTaxiRoute('Z3', 'A4KZAx30', { departureRunway: '22R' }), 'err')).toEqual([]);
  });

  it('reports the runway end the clearance leads to', () => {
    expect(computeTaxiRoute('Z3', 'A104LKZAx30').destinationRunway).toBe('04L');
    expect(computeTaxiRoute('Z3', 'A4KZAx30').destinationRunway).toBe('22R');
    expect(computeTaxiRoute('Z3', 'KZAx30').destinationRunway).toBeNull();
  });

  it('leaves clearances alone when the holding point is not first, or is the only item', () => {
    expect(legNames(computeTaxiRoute('Z3', 'KZAx30A4'))).toEqual(['K', 'Z', 'A', 'A4>']);
    expect(computeTaxiRoute('Z3', 'A4').parsed.items.map((i) => i.name)).toEqual(['A4']);
  });

  it('does not apply to arrivals', () => {
    expect(computeArrivalRoute('22L', 'F98', 'V1V').messages.some((m) => m.text.startsWith('Holding point'))).toBe(false);
  });
});

describe('G and runway 12/30 via G1', () => {
  it.each(['12', '30'])('holds short of %s via G1 after G', (end) => {
    const route = computeTaxiRoute('G6', `G.${end}`);
    expect(legNames(route)).toEqual(['G6', 'G', 'G1']);
    expect(marks(route)).toEqual([`hold:${end}`]);
    expect(levels(route, 'info')).toContain(`Joined ${end} via G1 (standard routing)`);
  });

  it('taxis from G onto runway 30 via G1', () => {
    const route = computeTaxiRoute('G6', 'G30');
    expect(legNames(route)).toEqual(['G6', 'G', 'G1', '30>']);
    expect(levels(route, 'info')).toContain('G to 30 via G1 (standard routing)');
  });

  it.each(['12', '30'])('vacates %s onto G via G1 on arrival', (runway) => {
    const route = computeArrivalRoute(runway, 'G126', 'G');
    expect(legNames(route)).toEqual(['G1', 'G>']);
    expect(levels(route, 'err')).toEqual([]);
    expect(levels(route, 'info')).toContain(`Vacating ${runway} onto G via G1 (standard routing)`);
  });

  it('does not route other taxiways via G1', () => {
    expect(levels(computeTaxiRoute('Z3', 'K.12'), 'info').some((m) => m.includes('G1'))).toBe(false);
  });
});

describe('impliedDepartureRunway', () => {
  it.each([
    ['KZAx30A4.22R', '22R'],
    ['A4KZAx30', '22R'],
    ['KZAx30A4', '22R'],
    ['KZA.30A4', '22R'],
    ['Bx30B2.04R', '04R'],
    ['KZ.30', '30'],
    ['KZA', null],
    ['', null],
  ])('%s -> %s', (clearance, runway) => {
    expect(impliedDepartureRunway(computeTaxiRoute('Z3', clearance).parsed)).toBe(runway);
  });
});

describe('assigned departure runway written first', () => {
  const route = (clearance: string, departureRunway: string | null, rp = 'Z3') => computeTaxiRoute(rp, clearance, { departureRunway });

  it('rejects a leading reciprocal runway end that conflicts with the assignment', () => {
    const r = route('04LKZAx30', '22R');
    expect(r.parsed.items[0]).toMatchObject({ type: 'rwy', end: '04L' });
    expect(r.legs).toEqual([]);
    expect(levels(r, 'err')).toEqual(['Departure runway 04L in the clearance conflicts with assigned runway 22R.']);
  });

  it.each(['22RKZAx30A4', '.22RKZAx30A4', '22R K Z A x30 A4'])('treats %s as the destination', (clearance) => {
    const r = route(clearance, '22R');
    expect(legNames(r)).toEqual(['K', 'Z', 'A', 'A4']);
    expect(marks(r)).toEqual(['cross:30', 'hold:22R']);
    expect(levels(r, 'info')).toContain('Runway 22R written first is treated as the departure runway.');
    expect(levels(r, 'err')).toEqual([]);
  });

  it.each(['22R A4 KZAx30', 'A4 22R KZAx30', '.A4KZAx30'])('moves the runway and its holding point (%s) to the end', (clearance) => {
    const r = route(clearance, '22R');
    expect(legNames(r)).toEqual(['K', 'Z', 'A', 'A4']);
    expect(marks(r)).toEqual(['cross:30', 'hold:22R']);
  });

  it('applies the standard routings to the destination too', () => {
    const r = route('30G', '30', 'G6');
    expect(legNames(r)).toEqual(['G6', 'G', 'G1']);
    expect(marks(r)).toEqual(['hold:30']);
  });

  it('leaves a runway that is not the assigned one alone', () => {
    expect(route('30KZA', '22R').parsed.items[0]).toMatchObject({ type: 'rwy', name: '12/30' });
    expect(route('22RKZA', null).parsed.items[0]).toMatchObject({ type: 'rwy', name: '04L/22R' });
  });

  it('keeps ".A4" first as a hold short of A4 when no runway is assigned', () => {
    expect(route('.A4KZAx30', null).parsed.items[0]).toMatchObject({ type: 'hold', name: 'A4' });
  });
});

describe('departure runway joined from A (22R) and B (04R)', () => {
  const route = (rp: string, clearance: string, departureRunway: string | null) => computeTaxiRoute(rp, clearance, { departureRunway });

  it('keeps the onward route open after an explicit hold for the assigned runway', () => {
    const r = route('Z3', '22RKZAx30A4.22R/A', '22R');
    expect(r.parsed.items.filter((item) => item.type === 'hold')).toEqual([
      expect.objectContaining({ end: '22R', segment: 0 }),
    ]);
    expect(r.stub).toMatchObject({ name: 'A', past: false });
    expect(levels(r, 'err')).toEqual([]);
    expect(levels(r, 'ok')).toEqual(['Route OK. A is open-ended until the next taxiway.']);
    expect(levels(r, 'info')).toEqual(['Runway 22R written first is treated as the departure runway.']);
  });

  it('continues A via A1 to hold short of 22R', () => {
    const r = route('Z3', 'KZAx30', '22R');
    expect(legNames(r)).toEqual(['K', 'Z', 'A', 'A1']);
    expect(marks(r)).toEqual(['cross:30', 'hold:22R']);
    expect(levels(r, 'info')).toContain('No hold short given: continuing to hold short of runway 22R.');
  });

  it('continues B to 04R at the map edge', () => {
    const r = route('B', 'Bx30', '04R');
    expect(legNames(r)).toEqual(['B']);
    expect(marks(r)).toEqual(['cross:30', 'hold:04R']);
    // Nothing is drawn past the map edge.
    expect(Math.max(...r.legs[0].pts.map((p) => p[1]))).toBeLessThanOrEqual(2286.5);
  });

  it('only applies to the assigned runway, its taxiway, and clearances without a hold short', () => {
    expect(legNames(route('Z3', 'KZAx30', '04R'))).toEqual(['K', 'Z', 'A>']);
    expect(legNames(route('Z3', 'KZAx30', null))).toEqual(['K', 'Z', 'A>']);
    expect(marks(route('Z3', 'KZAx30.A4', '22R'))).toEqual(['cross:30', 'hold:A4']);
    expect(legNames(route('Z3', 'KZAx30A4', '22R'))).toEqual(['K', 'Z', 'A', 'A4>']);
  });

  it('lets B hold short of 04R when written out, and keeps B1-B4', () => {
    expect(marks(route('B', 'Bx30.04R', null))).toEqual(['cross:30', 'hold:04R']);
    expect(legNames(route('B', 'Bx30B2.04R', '04R'))).toEqual(['B', 'B2']);
  });
});

describe('conflicting hold and crossing', () => {
  it('keeps the hold short when the same clearance also crosses that runway', () => {
    const route = computeArrivalRoute('04L', 'F98', 'A.30x30Y');
    expect(route.marks.filter((mark) => mark.name === '12/30').map((mark) => mark.type)).toEqual(['hold']);
    expect(levels(route, 'warn')).toContain('Both hold short and cross 30?');
    expect(route.holdAt).not.toBeNull();
  });
});

describe('first hold along the route', () => {
  it.each(['KZAx30A4.22R', 'KZ.A/Ax12/30A4.22R', 'A.30x30Y', 'V1VY.L', 'KZA.30', 'G.12'])('stops at the earliest hold bar for %s', (clearance) => {
    const route = /^(A|V1)/.test(clearance) ? computeArrivalRoute(clearance.startsWith('A') ? '04L' : '22L', 'A21', clearance) : computeTaxiRoute(clearance.startsWith('G') ? 'G6' : 'Z3', clearance);
    const holds = route.marks.filter((mark) => mark.type === 'hold');
    if (!holds.length) return;
    const a = arcs(routePoints(route));
    expect(route.holdAt).toBe(Math.min(...holds.map((mark) => a[mark.g] - holdBack(mark))));
  });
});

describe('runway hold bar before an earlier taxiway hold', () => {
  it('stops at the runway bar on C, which comes before the hold short of F', () => {
    // Southbound on C, F joins at J19 and runway 12/30 at J21 just after it; the runway bar sits
    // further back from its junction, so it is reached first.
    const route = computeTaxiRoute('Z8', 'AC.FD');
    const a = arcs(routePoints(route));
    const stop = (name: string) => {
      const mark = route.marks.find((m) => m.type === 'hold' && m.name === name)!;
      return a[mark.g] - holdBack(mark);
    };
    expect(stop('12/30')).toBeLessThan(stop('F'));
    expect(route.holdAt).toBe(stop('12/30'));
  });
});
