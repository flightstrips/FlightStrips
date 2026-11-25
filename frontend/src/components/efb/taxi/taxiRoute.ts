// Turns a written taxi clearance into a drawable route over the EKCH taxi network.
//
// Clearance syntax (case and spaces are ignored):
//   KZA       taxiways in order
//   .22R ,Z   hold short of a runway end or a taxiway
//   x30       cleared to cross a runway ("cross 30" also works)
//   30        a runway on its own: taxi along it in that direction (30 = 300 deg)
import networkData from './ekch-taxi-network.json';

export type Point = [number, number];

export interface TaxiNetwork {
  airport: string;
  width: number;
  height: number;
  chart: { width: number; height: number };
  /** offMap: a taxiway beyond the map edge, drawn as a short stub past it (e.g. B1-B3 to 04R). */
  lines: Array<{ name: string; kind: 'taxiway' | 'runway'; ends?: string[]; offMap?: boolean }>;
  nodes: Array<{ id: string; x: number; y: number; names: string[]; holdFor?: string[] }>;
  edges: Array<{ from: string; to: string; name: string; kind: 'taxiway' | 'runway'; pts: Point[] }>;
  releasePoints: Array<{ name: string; x: number; y: number }>;
  /** Taxiways that reach a runway beyond the map edge, at node; arrivals may vacate onto them there. */
  runwayLinks?: Array<{ taxiway: string; runway: string; node: string }>;
  /** Stand positions; entries are where the stand joins the taxi network (one per taxiway it can be reached from). */
  stands: Array<{ name: string; x: number; y: number; entries: Array<{ x: number; y: number; taxiway: string }> }>;
}

export const EKCH_TAXI_NETWORK = networkData as TaxiNetwork;

// ---- airport-specific rules ------------------------------------------------------------------

/** Hold short of these runway ends when the last taxiway does not reach the runway: continue via this taxiway. */
const HOLD_SHORT_VIA: Record<string, string> = {
  '22R': 'A1', '22L': 'V1',
  // Only after G: G does not touch runway 12/30, G1 joins them.
  'G>12': 'G1', 'G>30': 'G1',
};
/** Standard routings between two taxiways written next to each other: "R>Y" means R then Y goes via V. */
const VIA_BETWEEN: Record<string, string> = {
  'R>Y': 'V',
  // G and runway 12/30 (either direction, and vacating it onto G) are joined by G1.
  'G>12/30': 'G1', '12/30>G': 'G1',
};
/** Cosmetic: when these taxiways appear in this order, draw the middle one as a smooth curve. */
const SMOOTH: Array<{ seq: [string, string, string]; trim: number }> = [{ seq: ['P', 'Y', 'A'], trim: 70 }];
/** Runway holding points (departures). Runway 30 has none. */
const HOLDING_POINTS: Record<string, string> = {
  A1: '22R', A2: '22R', A3: '22R', A4: '22R', A5: '22R',
  B1: '04R', B2: '04R', B3: '04R', B4: '04R',
  K1: '12', F1: '12',
};

/**
 * Where a departure ends up is often said first: "holding point A4, runway 22R, via K Z A". Written
 * first, these are not part of the taxi route but its destination, so they move to the end:
 * - a holding point (A4KZA), optionally followed by its runway (A122R, .A122R);
 * - the assigned departure runway (22R or .22R), optionally with one of its holding points.
 * The route then holds short of that runway (unless the clearance already does). Without an
 * assigned runway, a runway written first still means taxiing along it.
 */
function moveLeadingDestination(parsed: ParsedClearance, net: TaxiNetwork, assignedRunway: string | null): { parsed: ParsedClearance; point: string | null; runway: string | null; destination?: string | null; conflict?: string } {
  const unchanged = { parsed, point: null, runway: null };
  const tokens = vocabulary(net).runwayTokens;
  const assigned = assignedRunway ? findRunway(assignedRunway, net) : null;
  let holdingPoint: ClearanceItem | null = null, runwayItem: ClearanceItem | null = null, used = 0;
  for (const item of parsed.items.slice(0, 2)) {
    const hpRunway = holdingPoint ? tokens[HOLDING_POINTS[holdingPoint.name]] : null;
    if (!holdingPoint && (item.type === 'twy' || item.type === 'hold') && HOLDING_POINTS[item.name]
      && (!runwayItem || tokens[HOLDING_POINTS[item.name]] === runwayItem.name)) holdingPoint = item;
    else if (!runwayItem && (item.type === 'rwy' || item.type === 'hold') && item.end !== undefined
      && (item.name === hpRunway || item.name === assigned?.name)) runwayItem = item;
    else break;
    used++;
  }
  if (!used) return unchanged;
  // ".A1" on its own is a hold short of A1, unless A1 is a holding point of the assigned runway.
  if (holdingPoint?.type === 'hold' && !runwayItem && tokens[HOLDING_POINTS[holdingPoint.name]] !== assigned?.name) return unchanged;
  const rest = parsed.items.slice(used);
  if (!rest.some((item) => item.type === 'twy' || item.type === 'rwy')) return unchanged;
  // The runway end the clearance leads to: as written, otherwise the one its holding point serves.
  const impliedEnd = runwayItem?.end ?? (holdingPoint ? HOLDING_POINTS[holdingPoint.name] : null);
  if (assigned && assigned.end !== assigned.name && impliedEnd && tokens[impliedEnd] === assigned.name && impliedEnd !== assigned.end) {
    return {
      ...unchanged,
      conflict: runwayItem?.end
        ? `Departure runway ${runwayItem.end} in the clearance conflicts with assigned runway ${assigned.end}.`
        : `Holding point ${holdingPoint!.name} is for runway ${impliedEnd}, which conflicts with assigned runway ${assigned.end}.`,
    };
  }

  const active = parsed.activeSegment;
  const runway = runwayItem?.name ?? (holdingPoint ? tokens[HOLDING_POINTS[holdingPoint.name]] : assigned!.name);
  const end = runwayItem?.end ?? (holdingPoint ? HOLDING_POINTS[holdingPoint.name] : assigned!.end);
  const items: ClearanceItem[] = [...rest];
  if (holdingPoint) {
    // The holding point ends the clearance it was written in: taxi to it before that clearance's
    // hold short of its runway, so an onward clearance starts from the holding point.
    const segment = holdingPoint.segment;
    const point: ClearanceItem = { type: 'twy', name: holdingPoint.name, text: holdingPoint.name, segment };
    const runwayHold = items.findIndex((item) => item.type === 'hold' && item.segment === segment && item.name === runway);
    const afterSegment = items.findIndex((item) => item.segment > segment);
    const at = runwayHold >= 0 ? runwayHold : afterSegment >= 0 ? afterSegment : items.length;
    items.splice(at, 0, point);
  }
  const holds = items.filter((item) => item.type === 'hold');
  if (runway && !holds.some((item) => item.name === runway)
    && (runwayItem || !holds.some((item) => item.segment === active))) {
    items.push({ type: 'hold', name: runway, end, text: `.${end}`, segment: active });
  }
  return { parsed: { ...parsed, items }, point: holdingPoint?.name ?? null, runway: runwayItem ? end : null, destination: end ?? null };
}

/** With this departure runway assigned and no hold short given, a clearance ending on these taxiways goes on to hold short of the runway. */
const DEPARTURE_RUNWAY_JOINS: Record<string, string[]> = { '22R': ['A'], '04R': ['B'] };

function addDepartureRunwayHold(parsed: ParsedClearance, net: TaxiNetwork, departureRunway: string | null): { parsed: ParsedClearance; added: string | null } {
  const runway = findRunway(departureRunway, net);
  const joins = runway ? DEPARTURE_RUNWAY_JOINS[runway.end] : undefined;
  if (!runway || !joins) return { parsed, added: null };
  if (parsed.items.some((item) => item.type === 'hold' && (item.name === runway.name || item.segment === parsed.activeSegment))) {
    return { parsed, added: null };
  }
  const active = parsed.items.filter((item) => item.segment === parsed.activeSegment);
  const last = [...active].reverse().find((item) => item.type === 'twy' || item.type === 'rwy');
  if (!last || last.type !== 'twy' || !joins.includes(last.name)) return { parsed, added: null };
  const hold: ClearanceItem = { type: 'hold', name: runway.name, end: runway.end, text: `.${runway.end}`, segment: parsed.activeSegment };
  return { parsed: { ...parsed, items: [...parsed.items, hold] }, added: runway.end };
}

const STUB_LENGTH = 160;
export const MAX_CLEARANCE_LENGTH = 512;

// ---- parsing ---------------------------------------------------------------------------------

export type ClearanceItemType = 'twy' | 'hold' | 'cross' | 'rwy';
export interface ClearanceItem {
  type: ClearanceItemType;
  /** Taxiway name, or the full runway name (e.g. "04L/22R"). */
  name: string;
  /** The runway end as written (e.g. "22R"); null when the full runway name was written. */
  end?: string | null;
  text: string;
  /** Onward clearances: 0 for the first part, +1 after each "/". */
  segment: number;
}
export interface ParsedClearance {
  items: ClearanceItem[];
  bad: string;
  overLimit?: boolean;
  /** Index of the active (last) part; items in earlier parts are done (drawn grey). */
  activeSegment: number;
}

interface Vocabulary { taxiways: string[]; runwayTokens: Record<string, string> }

const vocabularyCache = new WeakMap<TaxiNetwork, Vocabulary>();
function vocabulary(net: TaxiNetwork): Vocabulary {
  let vocab = vocabularyCache.get(net);
  if (!vocab) {
    const runwayTokens: Record<string, string> = {};
    for (const line of net.lines) {
      if (line.kind !== 'runway') continue;
      runwayTokens[line.name] = line.name;
      for (const end of line.ends ?? []) runwayTokens[end] = line.name;
    }
    const taxiways = [...new Set(net.lines.filter((line) => line.kind !== 'runway').map((line) => line.name))].sort((a, b) => b.length - a.length);
    vocab = { taxiways, runwayTokens };
    vocabularyCache.set(net, vocab);
  }
  return vocab;
}

export function parseClearance(raw: string, net: TaxiNetwork = EKCH_TAXI_NETWORK): ParsedClearance {
  if (raw.length > MAX_CLEARANCE_LENGTH) return { items: [], bad: '', activeSegment: 0, overLimit: true };
  const src = raw.toUpperCase().replace(/\s+/g, '');
  const { taxiways, runwayTokens } = vocabulary(net);
  const tokens = Object.keys(runwayTokens).sort((a, b) => b.length - a.length);
  const runwayAt = (i: number) => {
    const k = src.startsWith('RWY', i) ? i + 3 : i;
    const token = tokens.find((t) => src.startsWith(t, k));
    return token ? { name: runwayTokens[token], next: k + token.length, text: src.slice(i, k + token.length), end: token.includes('/') ? null : token } : null;
  };
  const taxiwaysAt = (i: number) => taxiways.filter((t) => src.startsWith(t, i)).map((t) => ({ name: t, next: i + t.length }));

  // "/" separates onward clearances. Runway names like 12/30 also contain "/"; the longest match wins.
  type RawItem = Omit<ClearanceItem, 'segment'> | { type: 'sep'; text: string };
  const memo = new Map<number, { items: RawItem[]; used: number }>();
  const at = (i: number): { items: RawItem[]; used: number } => {
    if (i >= src.length) return { items: [], used: 0 };
    const cached = memo.get(i);
    if (cached) return cached;
    const options: Array<{ item: RawItem; next: number }> = [];
    const c = src[i];
    if (c === '/') options.push({ item: { type: 'sep', text: '/' }, next: i + 1 });
    if (c === '.' || c === ',') {
      const runway = runwayAt(i + 1);
      if (runway) options.push({ item: { type: 'hold', name: runway.name, end: runway.end, text: c + runway.text }, next: runway.next });
      for (const t of taxiwaysAt(i + 1)) options.push({ item: { type: 'hold', name: t.name, text: c + t.name }, next: t.next });
    }
    for (const prefix of ['CROSS', 'X']) {
      if (!src.startsWith(prefix, i)) continue;
      const runway = runwayAt(i + prefix.length);
      if (runway) options.push({ item: { type: 'cross', name: runway.name, end: runway.end, text: prefix.toLowerCase() + runway.text }, next: runway.next });
    }
    const runway = runwayAt(i);
    if (runway) options.push({ item: { type: 'rwy', name: runway.name, end: runway.end, text: runway.text }, next: runway.next });
    for (const t of taxiwaysAt(i)) options.push({ item: { type: 'twy', name: t.name, text: t.name }, next: t.next });

    let best: { items: RawItem[]; used: number } = { items: [], used: 0 };
    for (const option of options) {
      const rest = at(option.next);
      const used = option.next - i + rest.used;
      if (used > best.used) best = { items: [option.item, ...rest.items], used };
      if (option.next + rest.used >= src.length) break;
    }
    memo.set(i, best);
    return best;
  };
  const best = at(0);
  const items: ClearanceItem[] = [];
  let segment = 0;
  for (const item of best.items) {
    if (item.type === 'sep') segment++;
    else items.push({ ...item, segment });
  }
  return { items, bad: src.slice(best.used), activeSegment: items.at(-1)?.segment ?? 0 };
}

export const itemLabel = (item: ClearanceItem) => item.end || item.name;

/** The runway line for a runway end such as "22R" (or a full name such as "04L/22R"). */
/**
 * The departure runway a clearance implies when the flight has none: its last holding point
 * (A4 -> 22R), otherwise its last hold short of a runway end (.22R). A hold short can also be a
 * runway to cross later, so holding points count first.
 */
export function impliedDepartureRunway(parsed: ParsedClearance): string | null {
  const items = [...parsed.items].reverse();
  const holdingPoint = items.find((item) => item.type === 'twy' && HOLDING_POINTS[item.name]);
  if (holdingPoint) return HOLDING_POINTS[holdingPoint.name];
  return items.find((item) => item.type === 'hold' && item.end)?.end ?? null;
}

export function findRunway(end: string | null | undefined, net: TaxiNetwork = EKCH_TAXI_NETWORK): { name: string; end: string } | null {
  const token = (end ?? '').trim().toUpperCase().replace(/^RWY/, '');
  const name = token ? vocabulary(net).runwayTokens[token] : undefined;
  return name ? { name, end: token } : null;
}

// ---- geometry --------------------------------------------------------------------------------

export const dist = (a: Point, b: Point) => Math.hypot(a[0] - b[0], a[1] - b[1]);
export function arcs(p: Point[]): number[] {
  const out = [0];
  for (let i = 1; i < p.length; i++) out.push(out[i - 1] + dist(p[i - 1], p[i]));
  return out;
}
export function atArc(p: Point[], a: number[], s: number): Point {
  s = Math.max(0, Math.min(a[a.length - 1], s));
  let i = 1;
  while (i < p.length - 1 && a[i] < s) i++;
  const seg = a[i] - a[i - 1] || 1, t = (s - a[i - 1]) / seg;
  return [p[i - 1][0] + t * (p[i][0] - p[i - 1][0]), p[i - 1][1] + t * (p[i][1] - p[i - 1][1])];
}
export function sliceArc(p: Point[], a: number[], s0: number, s1: number): Point[] {
  const out: Point[] = [atArc(p, a, s0)];
  for (let i = 0; i < p.length; i++) if (a[i] > s0 && a[i] < s1) out.push(p[i]);
  out.push(atArc(p, a, s1));
  return out;
}
export function unit(a: Point, b: Point): Point {
  const d = dist(a, b) || 1;
  return [(b[0] - a[0]) / d, (b[1] - a[1]) / d];
}
const polyLength = (p: Point[]) => arcs(p)[p.length - 1] ?? 0;

// ---- graph -----------------------------------------------------------------------------------

interface Node { id: string; x: number; y: number; names: Set<string>; holdFor?: string[] }
interface Edge { i: number; from: string; to: string; name: string; kind: string; pts: Point[]; len: number }
interface Graph { nodes: Record<string, Node>; edges: Edge[]; adj: Record<string, number[]>; splits: Record<string, { name: string; point: Point }> }

/** A point placed onto the nearest taxiway (optionally a given one), splitting that edge at a new node. */
interface Split { id: string; x: number; y: number; taxiway?: string }

function buildGraph(net: TaxiNetwork, splits: Split[]): Graph {
  const nodes: Record<string, Node> = {};
  for (const n of net.nodes) nodes[n.id] = { id: n.id, x: n.x, y: n.y, names: new Set(n.names), holdFor: n.holdFor };
  let raw = net.edges.map((e) => ({ from: e.from, to: e.to, name: e.name, kind: e.kind as string, pts: e.pts }));
  const placed: Graph['splits'] = {};
  for (const sp of splits) {
    let best: { d: number; ei: number; i: number; c: Point } | null = null;
    raw.forEach((e, ei) => {
      if (e.kind === 'runway' || (sp.taxiway && e.name !== sp.taxiway)) return;
      for (let i = 0; i < e.pts.length - 1; i++) {
        const a = e.pts[i], b = e.pts[i + 1], dx = b[0] - a[0], dy = b[1] - a[1], l2 = dx * dx + dy * dy;
        const t = l2 ? Math.max(0, Math.min(1, ((sp.x - a[0]) * dx + (sp.y - a[1]) * dy) / l2)) : 0;
        const c: Point = [a[0] + t * dx, a[1] + t * dy], d = dist(c, [sp.x, sp.y]);
        if (!best || d < best.d) best = { d, ei, i, c };
      }
    });
    const hit = best as { d: number; ei: number; i: number; c: Point } | null;
    if (!hit) continue;
    const e = raw[hit.ei];
    nodes[sp.id] = { id: sp.id, x: hit.c[0], y: hit.c[1], names: new Set([e.name]) };
    placed[sp.id] = { name: e.name, point: hit.c };
    raw = [
      ...raw.slice(0, hit.ei),
      { ...e, to: sp.id, pts: [...e.pts.slice(0, hit.i + 1), hit.c] },
      { ...e, from: sp.id, pts: [hit.c, ...e.pts.slice(hit.i + 1)] },
      ...raw.slice(hit.ei + 1),
    ];
  }
  const edges: Edge[] = [];
  const adj: Record<string, number[]> = {};
  for (const e of raw) {
    const edge = { ...e, i: edges.length, len: polyLength(e.pts) };
    edges.push(edge);
    (adj[e.from] ??= []).push(edge.i);
    (adj[e.to] ??= []).push(edge.i);
  }
  return { nodes, edges, adj, splits: placed };
}

const other = (e: Edge, n: string) => (e.from === n ? e.to : e.from);
const ptsFrom = (e: Edge, n: string): Point[] => (e.from === n ? e.pts : [...e.pts].reverse());

interface Step { e: Edge; from: string }
type EdgeFilter = (e: Edge, from: string) => boolean;

function search(g: Graph, src: string, goal: (n: string) => boolean, ok: EdgeFilter, allowSrc = false): { node: string; steps: Step[] } | null {
  if (allowSrc && goal(src)) return { node: src, steps: [] };
  const d: Record<string, number> = { [src]: 0 };
  const prev: Record<string, { e: number; n: string }> = {};
  const done = new Set<string>();
  const queue = [src];
  while (queue.length) {
    queue.sort((a, b) => d[a] - d[b]);
    const u = queue.shift()!;
    if (done.has(u)) continue;
    done.add(u);
    if (u !== src && goal(u)) {
      const steps: Step[] = [];
      for (let x = u; x !== src;) { const p = prev[x]; steps.unshift({ e: g.edges[p.e], from: p.n }); x = p.n; }
      return { node: u, steps };
    }
    for (const ei of g.adj[u] ?? []) {
      const e = g.edges[ei];
      if (!ok(e, u)) continue;
      const v = other(e, u), nd = d[u] + e.len;
      if (d[v] === undefined || nd < d[v]) { d[v] = nd; prev[v] = { e: ei, n: u }; queue.push(v); }
    }
  }
  return null;
}

function walk(g: Graph, n: string, first: Edge, name: string, length: number): Pick<RouteLeg, 'pts' | 'nodes'> {
  const pts: Point[] = [], nodes: RouteLeg['nodes'] = [];
  let node = n, e: Edge | null = first, left = length;
  while (e && left > 0) {
    const p = ptsFrom(e, node);
    if (!pts.length) pts.push(p[0]);
    for (let i = 1; i < p.length; i++) {
      const sl = dist(p[i - 1], p[i]);
      if (sl > left) {
        const t = left / sl;
        pts.push([p[i - 1][0] + t * (p[i][0] - p[i - 1][0]), p[i - 1][1] + t * (p[i][1] - p[i - 1][1])]);
        return { pts, nodes };
      }
      left -= sl;
      pts.push(p[i]);
      if (left === 0 && i < p.length - 1) return { pts, nodes };
    }
    const next = other(e, node), dir = unit(p[p.length - 2], p[p.length - 1]);
    nodes.push({ n: next, k: pts.length - 1 });
    let best: { c: number; f: Edge } | null = null;
    for (const ei of g.adj[next] ?? []) {
      const f = g.edges[ei];
      if (f === e || f.name !== name) continue;
      const q = ptsFrom(f, next), d2 = unit(q[0], q[1]), c = dir[0] * d2[0] + dir[1] * d2[1];
      if (!best || c > best.c) best = { c, f };
    }
    node = next;
    e = best && best.c > 0 ? best.f : null;
  }
  return { pts, nodes };
}

function reach(g: Graph, n: string, first: Edge, name: string): number {
  const seen = new Set([first.i]), visited = new Set([n]), stack = [other(first, n)];
  let length = first.len;
  while (stack.length) {
    const u = stack.pop()!;
    if (visited.has(u)) continue;
    visited.add(u);
    for (const ei of g.adj[u] ?? []) {
      const e = g.edges[ei];
      if (e.name !== name || seen.has(ei)) continue;
      seen.add(ei);
      length += e.len;
      stack.push(other(e, u));
    }
  }
  return length;
}

// ---- route -----------------------------------------------------------------------------------

export interface RouteLeg {
  name: string;
  segment: number;
  pts: Point[];
  /** Junctions reached along this leg, with their index in pts. */
  nodes: Array<{ n: string; k: number }>;
  connector?: boolean;
  runway?: boolean;
  auto?: boolean;
  /** Part of an earlier clearance, before the last "/" (drawn grey). */
  past?: boolean;
  /** The taxiway continues beyond the map edge (drawn dashed). */
  offMap?: boolean;
}
export interface RouteMark {
  type: 'hold' | 'cross';
  /** Full line name (taxiway, or runway such as "12/30"). */
  name: string;
  /** Text to show, e.g. the runway end as written. */
  label: string;
  runway: boolean;
  implicit?: boolean;
  /** The node itself is a mapped hold-short point, rather than the runway centreline. */
  atHoldPoint?: boolean;
  /** Index into the concatenated route points. */
  g: number;
}
export type RouteMessageLevel = 'ok' | 'info' | 'warn' | 'err';
export interface TaxiRoute {
  mode: 'departure' | 'arrival';
  releasePoint: TaxiNetwork['releasePoints'][number] | null;
  start: Point | null;
  /** Text next to the start marker: the release point, or the runway and exit for arrivals. */
  startLabel: string | null;
  /** Arrival destination stand, if known. */
  stand: TaxiNetwork['stands'][number] | null;
  /** The route ends at the stand (drawn with a link from the taxiway into the stand). */
  standReached: boolean;
  /** Where the route joins the stand (one of its entries), once reached. */
  standEntry: Point | null;
  parsed: ParsedClearance;
  legs: RouteLeg[];
  stub: (RouteLeg & { choices: number }) | null;
  marks: RouteMark[];
  /** Arc position (along the concatenated legs) of the first hold short; beyond it the route is dimmed. */
  holdAt: number | null;
  /** Departures: the runway end a runway or holding point written first leads to, as the clearance names it. */
  destinationRunway: string | null;
  messages: Array<{ level: RouteMessageLevel; text: string }>;
}

function legFrom(steps: Step[], extra: Omit<RouteLeg, 'pts' | 'nodes'>): RouteLeg {
  const pts: Point[] = [], nodes: RouteLeg['nodes'] = [];
  for (const st of steps) {
    const p = ptsFrom(st.e, st.from);
    pts.push(...(pts.length ? p.slice(1) : p));
    nodes.push({ n: other(st.e, st.from), k: pts.length - 1 });
  }
  return { pts, nodes, ...extra };
}

export function findReleasePoint(name: string | null | undefined, net: TaxiNetwork = EKCH_TAXI_NETWORK) {
  const wanted = (name ?? '').trim().toUpperCase();
  return wanted ? net.releasePoints.find((rp) => rp.name === wanted) ?? null : null;
}

export function findStand(name: string | null | undefined, net: TaxiNetwork = EKCH_TAXI_NETWORK) {
  const wanted = (name ?? '').trim().toUpperCase();
  return wanted ? net.stands.find((stand) => stand.name === wanted) ?? null : null;
}

interface RouteOptions {
  flip?: boolean;
  net?: TaxiNetwork;
  /** Departures: the assigned runway (flight or pilot). Written first in the clearance, it is the destination. */
  departureRunway?: string | null;
}
type RouteStart = { kind: 'departure'; releasePoint: string } | { kind: 'arrival'; runway: string; stand: string };

/** Departure: from the release point along the cleared taxiways. */
export function computeTaxiRoute(releasePointName: string, clearance: string, options: RouteOptions = {}): TaxiRoute {
  return computeRoute({ kind: 'departure', releasePoint: releasePointName }, clearance, options);
}

/** Arrival: from where the first cleared taxiway leaves the landing runway, to the stand. */
export function computeArrivalRoute(runway: string, stand: string, clearance: string, options: RouteOptions = {}): TaxiRoute {
  return computeRoute({ kind: 'arrival', runway, stand }, clearance, options);
}

function computeRoute(spec: RouteStart, clearance: string, options: RouteOptions): TaxiRoute {
  const net = options.net ?? EKCH_TAXI_NETWORK;
  const destination = spec.kind === 'departure' ? moveLeadingDestination(parseClearance(clearance, net), net, options.departureRunway ?? null) : null;
  const runwayHold = destination && !destination.conflict ? addDepartureRunwayHold(destination.parsed, net, options.departureRunway ?? null) : null;
  const parsed = runwayHold?.parsed ?? destination?.parsed ?? parseClearance(clearance, net);
  const messages: TaxiRoute['messages'] = [];
  if (runwayHold?.added) messages.push({ level: 'info', text: `No hold short given: continuing to hold short of runway ${runwayHold.added}.` });
  if (destination?.point) messages.push({ level: 'info', text: `Holding point ${destination.point} written first: taxiing to it at the end of the route.` });
  else if (destination?.runway) messages.push({ level: 'info', text: `Runway ${destination.runway} written first is treated as the departure runway.` });
  const res: TaxiRoute = {
    mode: spec.kind, releasePoint: null, start: null, startLabel: null, stand: null, standReached: false, standEntry: null,
    parsed, legs: [], stub: null, marks: [], holdAt: null, destinationRunway: destination?.destination ?? null, messages,
  };
  const say = (level: RouteMessageLevel, text: string) => messages.push({ level, text });
  if (parsed.overLimit) {
    say('err', `Limit the clearance to ${MAX_CLEARANCE_LENGTH} characters.`);
    return res;
  }
  if (destination?.conflict) {
    say('err', destination.conflict);
    return res;
  }
  const isRunway = (name: string) => net.lines.some((l) => l.name === name && l.kind === 'runway');

  // Route elements, with standard "via" taxiways inserted.
  const route: ClearanceItem[] = [];
  for (const item of parsed.items) {
    if (item.type !== 'twy' && item.type !== 'rwy') continue;
    const prev = route[route.length - 1];
    const via = prev && VIA_BETWEEN[`${prev.name}>${item.name}`];
    if (via) { route.push({ type: 'twy', name: via, text: via, segment: item.segment }); say('info', `${itemLabel(prev)} to ${itemLabel(item)} via ${via} (standard routing)`); }
    route.push(item);
  }
  const names = route.map((item) => item.name);
  // Hold shorts in earlier parts (before the last "/") were released by the onward clearance.
  const activeHolds = parsed.items.filter((i) => i.type === 'hold' && i.segment === parsed.activeSegment);
  const holds = activeHolds.map((i) => i.name);
  const label = Object.fromEntries(activeHolds.map((item) => [item.name, itemLabel(item)]));
  const isPast = (i: number) => route[i].segment < parsed.activeSegment;
  const crosses = parsed.items.filter((i) => i.type === 'cross');
  const disp = (i: number) => itemLabel(route[i]);
  const legSteps: Step[][] = [];
  let stubEdge: Edge | undefined;
  const appendLeg = (steps: Step[], segment: number, extra: Omit<RouteLeg, 'pts' | 'nodes' | 'segment' | 'past'>) => {
    legSteps.push(steps);
    res.legs.push(legFrom(steps, { ...extra, segment, past: segment < parsed.activeSegment }));
  };

  if (parsed.bad) say('err', `Can't read "${parsed.bad}"`);
  for (const c of crosses) if (c.segment === parsed.activeSegment && holds.includes(c.name)) say('warn', `Both hold short and cross ${itemLabel(c)}?`);
  for (const item of route) if (item.type === 'rwy' && !item.end) say('err', `Say which direction to taxi on ${item.name}: ${item.name.split('/').join(' or ')}`);

  // Where the route starts (and, for arrivals, the stand it ends at).
  const splits: Split[] = [];
  let landing: { name: string; end: string; heading: Point } | null = null;
  if (spec.kind === 'departure') {
    const rpName = spec.releasePoint.trim().toUpperCase();
    const rp = findReleasePoint(rpName, net);
    if (!rp) { say(rpName ? 'err' : 'warn', rpName ? `Unknown release point "${rpName}"` : 'Enter the release point'); return res; }
    res.releasePoint = rp;
    res.startLabel = rp.name;
    splits.push({ id: 'START', x: rp.x, y: rp.y });
  } else {
    const standName = spec.stand.trim().toUpperCase();
    res.stand = findStand(standName, net);
    if (standName && !res.stand) say('warn', `Stand ${standName} is not on the taxi map`);
    res.stand?.entries.forEach((entry, i) => splits.push({ id: `STAND${i}`, x: entry.x, y: entry.y, taxiway: entry.taxiway }));
    const end = spec.runway.trim().toUpperCase().replace(/^RWY/, '');
    const name = vocabulary(net).runwayTokens[end];
    if (!end) { say('warn', 'Enter the landing runway'); return res; }
    if (!name || end.includes('/')) { say('err', `Unknown runway "${end}"`); return res; }
    const a = parseInt(end, 10) * 10 * Math.PI / 180;
    landing = { name, end, heading: [Math.sin(a), -Math.cos(a)] };
  }
  const g = buildGraph(net, splits);
  if (spec.kind === 'departure') res.start = g.splits.START?.point ?? null;
  if (!route.length) {
    if (spec.kind === 'arrival') say('info', `The first taxiway is where you vacate runway ${landing!.end}.`);
    return res;
  }

  const on = (n: string, name: string) => g.nodes[n]?.names.has(name) ?? false;
  const onHold = (n: string, name: string) => on(n, name) || g.nodes[n]?.holdFor?.includes(name) === true;
  // A runway written on its own fixes the direction of travel: 30 -> heading 300 (map is north-up).
  const heading = (item: ClearanceItem): Point | null => {
    if (item.type !== 'rwy' || !item.end) return null;
    const a = parseInt(item.end, 10) * 10 * Math.PI / 180;
    return [Math.sin(a), -Math.cos(a)];
  };
  const along = (e: Edge, u: string, h: Point) => { const p = ptsFrom(e, u); return (p[p.length - 1][0] - p[0][0]) * h[0] + (p[p.length - 1][1] - p[0][1]) * h[1]; };
  const okFor = (item: ClearanceItem): EdgeFilter => { const h = heading(item); return (e, u) => e.name === item.name && (!h || along(e, u, h) > 0); };

  let cur = 'START';
  // Hold short already reported as unreachable via its standard taxiway (read by finish()).
  let warned: string | null = null;
  if (spec.kind === 'departure') {
    const startName = g.splits.START.name;
    if (startName !== names[0]) {
      const c = search(g, cur, (n) => on(n, names[0]), (e) => e.kind !== 'runway' && e.name === startName);
      if (!c) { say('err', `${res.releasePoint!.name} is on ${startName}, which does not join ${disp(0)}`); return finish(); }
      appendLeg(c.steps, route[0].segment, { name: startName, connector: true });
      cur = c.node;
    }
  } else {
    // Vacate the landing runway onto the first taxiway; with several junctions take the first one
    // reached on the landing roll.
    const { name, end, heading: h } = landing!;
    const viaExit = VIA_BETWEEN[`${name}>${names[0]}`];
    if (viaExit) {
      route.unshift({ type: 'twy', name: viaExit, text: viaExit, segment: route[0].segment });
      names.unshift(viaExit);
      say('info', `Vacating ${end} onto ${names[1]} via ${viaExit} (standard routing)`);
    }
    const linked = new Set((net.runwayLinks ?? []).filter((l) => l.runway === name && l.taxiway === names[0]).map((l) => l.node));
    const exits = Object.values(g.nodes).filter((n) => (n.names.has(name) && n.names.has(names[0])) || linked.has(n.id))
      .sort((p, q) => (p.x * h[0] + p.y * h[1]) - (q.x * h[0] + q.y * h[1]));
    if (!exits.length) { say('err', `${disp(0)} does not leave runway ${end}`); return res; }
    cur = exits[0].id;
    res.start = [exits[0].x, exits[0].y];
    res.startLabel = `${end} · ${disp(0)}`;
  }
  function continueFromPriorHold(item: ClearanceItem, onwardSegment: number): number {
    if (onwardSegment === item.segment) return item.segment;
    const segmentHold = parsed.items.find((hold) => hold.type === 'hold' && hold.segment === item.segment
      && (onHold(cur, hold.name) || !res.legs.some((leg) => leg.segment === item.segment && leg.nodes.some((node) => onHold(node.n, hold.name)))));
    if (!segmentHold) return item.segment;
    const toSegmentHold = search(g, cur, (n) => onHold(n, segmentHold.name), okFor(item), true);
    if (!toSegmentHold) return item.segment;

    // Movement up to the hold belongs to the earlier clearance. Movement from it can reuse the same taxiway.
    if (toSegmentHold.steps.length) {
      appendLeg(toSegmentHold.steps, item.segment, { name: itemLabel(item), runway: item.type === 'rwy' });
    }
    cur = toSegmentHold.node;
    return onwardSegment;
  }

  for (let i = 0; i < route.length - 1; i++) {
    const segment = continueFromPriorHold(route[i], route[i + 1].segment);
    const r = search(g, cur, (n) => on(n, names[i + 1]), okFor(route[i]), true);
    if (!r) {
      say('err', route[i].type === 'rwy' ? `Heading ${disp(i)} on the runway does not reach ${disp(i + 1)}` : `${disp(i)} does not connect to ${disp(i + 1)}`);
      return finish(route[i]);
    }
    if (r.steps.length) appendLeg(r.steps, segment, { name: disp(i), runway: route[i].type === 'rwy' });
    cur = r.node;
  }

  const lastIndex = route.length - 1, lastItem = route[lastIndex], lastName = names[lastIndex];
  const lastHeading = heading(lastItem), okLast = okFor(lastItem);
  const reached = (name: string) => res.legs.some((l) => !l.past && l.nodes.some((x) => onHold(x.n, name)));
  const pending = holds.filter((h) => !reached(h));
  const approachSegment = pending.length ? continueFromPriorHold(lastItem, parsed.activeSegment) : lastItem.segment;
  const atJunction = res.legs.at(-1)?.nodes.at(-1)?.n === cur;
  const toHold = pending.length ? search(g, cur, (n) => onHold(n, pending[0]), okLast, atJunction) : null;
  if (toHold) {
    if (toHold.steps.length) appendLeg(toHold.steps, approachSegment, { name: disp(lastIndex), runway: lastItem.type === 'rwy' });
  } else if (pending.length && (HOLD_SHORT_VIA[`${lastName}>${label[pending[0]]}`] ?? HOLD_SHORT_VIA[label[pending[0]]])) {
    const via = HOLD_SHORT_VIA[`${lastName}>${label[pending[0]]}`] ?? HOLD_SHORT_VIA[label[pending[0]]];
    const a = search(g, cur, (n) => on(n, via), okLast, true);
    const b = a && search(g, a.node, (n) => onHold(n, pending[0]), (e) => e.name === via);
    if (a && b) {
      if (a.steps.length) appendLeg(a.steps, approachSegment, { name: disp(lastIndex), runway: lastItem.type === 'rwy' });
      appendLeg(b.steps, parsed.activeSegment, { name: via, auto: true });
      say('info', `Joined ${label[pending[0]]} via ${via} (standard routing)`);
    } else {
      say('warn', `Hold short ${label[pending[0]]}: ${disp(lastIndex)} does not lead to ${via}`);
      warned = pending[0];
    }
  }
  // Arrivals end at the stand once the last taxiway is the one the stand joins.
  if (!toHold && !res.legs.some((l) => l.auto) && res.stand && !pending.length) {
    const toStand = search(g, cur, (n) => n.startsWith('STAND'), okLast, true);
    if (toStand) {
      if (toStand.steps.length) appendLeg(toStand.steps, lastItem.segment, { name: disp(lastIndex), runway: lastItem.type === 'rwy' });
      res.standReached = true;
      res.standEntry = g.splits[toStand.node]?.point ?? null;
    } else {
      const via = res.stand.entries.map((entry) => entry.taxiway).join(' or ');
      say('info', `Stand ${res.stand.name} is on ${via}; the route reaches it once ${via} is in the clearance.`);
    }
  }
  if (!toHold && !res.legs.some((l) => l.auto) && !res.standReached) {
    const candidates = (g.adj[cur] ?? []).map((i) => g.edges[i]).filter((e) => okLast(e, cur))
      .map((e) => ({ e, length: reach(g, cur, e, lastName) })).sort((x, y) => y.length - x.length);
    if (!candidates.length) {
      say('err', lastHeading ? `Can't taxi heading ${disp(lastIndex)} from here` : `Route does not reach ${disp(lastIndex)}`);
    } else {
      const pick = candidates[(options.flip ? 1 : 0) % candidates.length];
      stubEdge = pick.e;
      res.stub = {
        ...walk(g, cur, pick.e, lastName, lastHeading ? STUB_LENGTH * 1.6 : STUB_LENGTH),
        name: disp(lastIndex), segment: lastItem.segment, runway: lastItem.type === 'rwy',
        choices: lastHeading ? 1 : candidates.length, past: isPast(lastIndex),
      };
    }
  }
  smooth(res, (id) => {
    const node = g.nodes[id];
    return [...node.names].some((name) => isRunway(name) || holds.includes(name)) || node.holdFor?.some((name) => holds.includes(name)) === true;
  });
  return finish(undefined, toHold?.node);

  function finish(stoppedAt?: ClearanceItem, terminalHoldNode?: string): TaxiRoute {
    const offMap = new Set(net.lines.filter((l) => l.offMap).map((l) => l.name));
    const pieces = res.stub ? [...res.legs, res.stub] : res.legs;
    for (const piece of pieces) {
      if (!offMap.has(piece.name)) continue;
      piece.offMap = true;
      say('info', `${piece.name} is beyond the edge of the map; the route is drawn to where it joins.`);
    }
    const pendingHolds = new Set(holds);
    const unusedCrossings = [...crosses];
    let offset = 0;
    pieces.forEach((leg, li) => {
      const incomingRunway = leg.runway ? findRunway(leg.name, net)?.name : undefined;
      const nextLeg = pieces[li + 1];
      const onwardSegment = nextLeg?.segment ?? leg.segment;
      const onwardEdge = nextLeg === res.stub ? stubEdge : legSteps[li + 1]?.[0]?.e;
      // Reversing along the incoming edge leaves the aircraft on the same side of the runway.
      const turnsBack = onwardEdge !== undefined && onwardEdge === legSteps[li]?.at(-1)?.e;
      const onwardRunway = nextLeg
        ? (nextLeg.runway ? findRunway(nextLeg.name, net)?.name : undefined)
        : (stoppedAt?.type === 'rwy' ? stoppedAt.name : undefined);
      for (const x of leg.nodes) {
        const endOfLeg = x === leg.nodes.at(-1);
        const atEnd = endOfLeg && !nextLeg && !res.stub;
        // At a segment boundary, permission to proceed through the junction belongs to the onward leg.
        const segment = endOfLeg ? onwardSegment : leg.segment;
        // A new hold can end movement on a taxiway from the earlier clearance.
        const activeHold = segment === parsed.activeSegment || (atEnd && x.n === terminalHoldNode);
        for (const name of g.nodes[x.n].names) {
          const explicitHold = activeHold && pendingHolds.has(name);
          if (isRunway(name)) {
            if (!explicitHold && (name === incomingRunway || (endOfLeg && (turnsBack || name === onwardRunway)))) continue;
            // An active hold short outranks a crossing clearance for the same runway: keep the hold
            // and leave the crossing unused, so both conflicting instructions are reported.
            const crossingIndex = atEnd || explicitHold ? -1 : unusedCrossings.findIndex((item) => item.segment === segment && item.name === name);
            if (crossingIndex >= 0) {
              const [crossing] = unusedCrossings.splice(crossingIndex, 1);
              res.marks.push({ type: 'cross', name, label: itemLabel(crossing), runway: true, g: offset + x.k });
              continue;
            }
            res.marks.push({ type: 'hold', name, label: explicitHold ? label[name] : name, runway: true, implicit: !explicitHold, g: offset + x.k });
          } else if (explicitHold) {
            res.marks.push({ type: 'hold', name, label: label[name], runway: false, g: offset + x.k });
          }
          if (explicitHold) pendingHolds.delete(name);
        }
        if (activeHold) {
          for (const name of g.nodes[x.n].holdFor ?? []) {
            if (!pendingHolds.delete(name)) continue;
            res.marks.push({ type: 'hold', name, label: label[name], runway: true, atHoldPoint: true, g: offset + x.k });
          }
        }
      }
      offset += leg.pts.length;
    });
    for (const h of pendingHolds) if (warned !== h) say('warn', `Hold short ${label[h]}: not on the route yet`);
    for (const c of unusedCrossings) say('warn', `Cross ${itemLabel(c)}: the route does not cross it${res.stub ? ' yet' : ''}`);
    for (const m of res.marks) if (m.type === 'hold' && m.implicit) say('err', `No crossing clearance for ${m.name}. Hold short. (Add x${m.name.split('/').pop()} to cross.)`);
    // The first hold is the one whose bar is reached first: a runway bar sits further back from its
    // junction than a taxiway bar, so it can come before a taxiway hold at an earlier junction.
    let firstHold: RouteMark | undefined;
    const holdMarks = res.marks.filter((m) => m.type === 'hold');
    if (holdMarks.length) {
      const a = arcs(routePoints(res));
      const stops = holdMarks.map((mark) => ({ mark, at: a[mark.g] - holdBack(mark) })).sort((x, y) => x.at - y.at);
      firstHold = stops[0].mark;
      res.holdAt = stops[0].at;
    }
    if (!messages.some((m) => m.level === 'err')) {
      let text = 'Route OK.';
      if (res.standReached) {
        text = firstHold ? `Hold short of ${firstHold.runway ? 'runway' : 'taxiway'} ${firstHold.label}.` : `Route OK. Taxi to stand ${res.stand!.name}.`;
      } else if (res.stub) {
        text = `Route OK. ${res.stub.name} is open-ended until the next taxiway.`;
      }
      messages.unshift({ level: 'ok', text });
    }
    return res;
  }
}

/** How far before the junction a hold-short bar sits (runway edge vs. taxiway junction). */
export const RUNWAY_HOLD_BACK = 62;
export const TAXIWAY_HOLD_BACK = 34;

export function holdBack(mark: RouteMark): number {
  return mark.atHoldPoint ? 0 : mark.runway ? RUNWAY_HOLD_BACK : TAXIWAY_HOLD_BACK;
}

export function routePoints(route: TaxiRoute): Point[] {
  const all: Point[] = [];
  for (const leg of route.legs) all.push(...leg.pts);
  if (route.stub) all.push(...route.stub.pts);
  return all;
}

// ---- cosmetic smoothing ----------------------------------------------------------------------

function cutStart(p: Point[], d: number) {
  const a = arcs(p), length = a[a.length - 1];
  d = Math.min(d, length * 0.6);
  const pts = sliceArc(p, a, d, length), first = a.findIndex((x) => x > d);
  return { pts, map: (k: number) => (a[k] <= d ? 0 : k === p.length - 1 ? pts.length - 1 : Math.min(pts.length - 1, 1 + k - first)) };
}
function cutEnd(p: Point[], d: number) {
  const a = arcs(p), length = a[a.length - 1];
  d = Math.min(d, length * 0.6);
  return sliceArc(p, a, 0, length - d);
}

function smooth(route: TaxiRoute, isMarked: (node: string) => boolean) {
  for (const rule of SMOOTH) {
    const [a, b, c] = rule.seq;
    for (let i = 0; i + 1 < route.legs.length; i++) {
      const la = route.legs[i], lb = route.legs[i + 1], lc = route.legs[i + 2] ?? route.stub;
      if (!lc || la.name !== a || lb.name !== b || lc.name !== c || la.pts.length < 2 || lc.pts.length < 2) continue;
      const head = cutEnd(la.pts, rule.trim), tail = cutStart(lc.pts, rule.trim);
      // Keep protected junctions on their original geometry so the hold offsets stay correct.
      if (la.nodes.some(({ n, k }) => isMarked(n) && dist(la.pts[k], head[Math.min(k, head.length - 1)]) > 0)
        || lb.nodes.some(({ n }) => isMarked(n))
        || lc.nodes.some(({ n, k }) => isMarked(n) && dist(lc.pts[k], tail.pts[tail.map(k)]) > 0)) continue;
      const p0 = head[head.length - 1], p3 = tail.pts[0];
      const t0 = unit(head[head.length - 2] ?? la.pts[0], p0), t3 = unit(p3, tail.pts[1] ?? lc.pts[lc.pts.length - 1]);
      const k = dist(p0, p3) * 0.55;
      const p1: Point = [p0[0] + t0[0] * k, p0[1] + t0[1] * k], p2: Point = [p3[0] - t3[0] * k, p3[1] - t3[1] * k];
      const curve: Point[] = [];
      for (let s = 0; s <= 24; s++) {
        const u = s / 24, v = 1 - u;
        curve.push([
          v * v * v * p0[0] + 3 * v * v * u * p1[0] + 3 * v * u * u * p2[0] + u * u * u * p3[0],
          v * v * v * p0[1] + 3 * v * v * u * p1[1] + 3 * v * u * u * p2[1] + u * u * u * p3[1],
        ]);
      }
      la.pts = head;
      for (const x of la.nodes) x.k = Math.min(x.k, la.pts.length - 1);
      lb.pts = curve;
      for (const x of lb.nodes) x.k = curve.length - 1;
      lc.pts = tail.pts;
      for (const x of lc.nodes) x.k = tail.map(x.k);
    }
  }
}
