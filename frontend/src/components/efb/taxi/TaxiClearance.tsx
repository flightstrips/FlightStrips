import { useEffect, useId, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react';
import chartUrl from '@/assets/efb/taxi/ekch-taxi-chart.png';
import {
  EKCH_TAXI_NETWORK, MAX_CLEARANCE_LENGTH, RUNWAY_HOLD_BACK, arcs, atArc, findRunway, holdBack, itemLabel, routePoints, sliceArc, unit,
  type Point, type TaxiRoute,
} from './taxiRoute';
import type { TaxiClearanceState } from './useTaxiClearance';
import { commonRoutesFor, type CommonRoute } from './commonRoutes';

const ROUTE = '#ff2bd6';
const ROUTE_DIM = '#7a4a72';
/** Preliminary (standard) routing that the pilot has not entered as their clearance. */
const PRELIMINARY = '#22d3ee';
const PRELIMINARY_DIM = '#2b6f7c';
/** Taxi already cleared in an earlier part of the clearance (before the last "/"). */
const PAST = '#d3d8df';
/** Taxiway name signs: black text on airport-sign yellow. */
const SIGN = '#e9e814';
/** Departure runway (the destination), drawn under the route. */
const RUNWAY_DESTINATION = '#ffd84a';
const HOLD = '#ff3b3b';
const CROSS = '#2fe07a';
const ROUTE_WIDTH = 14;
const CROSS_OFFSET = 30;
/** The aircraft marker artwork is ~60 map px; drawn at this fraction of it. */
const AIRCRAFT_SCALE = 0.45;

interface ViewBox { x: number; y: number; w: number; h: number }
const FULL_VIEW: ViewBox = { x: 0, y: 0, w: EKCH_TAXI_NETWORK.width, h: EKCH_TAXI_NETWORK.height };
const ZOOM_STEP = 1.2;

function zoomView(view: ViewBox, factor: number, [x, y]: Point = [view.x + view.w / 2, view.y + view.h / 2]): ViewBox {
  const w = Math.min(FULL_VIEW.w * 1.5, Math.max(200, view.w * factor));
  const scale = w / view.w;
  return { x: x - (x - view.x) * scale, y: y - (y - view.y) * scale, w, h: view.h * scale };
}

/** Frame these points with padding, never tighter than a minimum area (so a single point is readable). */
function fitView(points: Point[], minSize = 1400): ViewBox {
  if (!points.length) return FULL_VIEW;
  const xs = points.map((p) => p[0]), ys = points.map((p) => p[1]), pad = 300;
  const cx = (Math.min(...xs) + Math.max(...xs)) / 2, cy = (Math.min(...ys) + Math.max(...ys)) / 2;
  const w = Math.max(minSize, Math.max(...xs) - Math.min(...xs) + 2 * pad), h = Math.max(minSize * 0.6, Math.max(...ys) - Math.min(...ys) + 2 * pad);
  return { x: cx - w / 2, y: cy - h / 2, w, h };
}

/** The point on a runway closest to p. */
function nearestOnRunway(runwayEnd: string | null | undefined, p: Point): Point | null {
  const runway = findRunway(runwayEnd);
  if (!runway) return null;
  let best: { d: number; c: Point } | null = null;
  for (const edge of EKCH_TAXI_NETWORK.edges) {
    if (edge.name !== runway.name) continue;
    for (let i = 0; i < edge.pts.length - 1; i++) {
      const a = edge.pts[i], b = edge.pts[i + 1], dx = b[0] - a[0], dy = b[1] - a[1], l2 = dx * dx + dy * dy;
      const t = l2 ? Math.max(0, Math.min(1, ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / l2)) : 0;
      const c: Point = [a[0] + t * dx, a[1] + t * dy], d = Math.hypot(p[0] - c[0], p[1] - c[1]);
      if (!best || d < best.d) best = { d, c };
    }
  }
  return best?.c ?? null;
}

const polyline = (pts: Point[]) => pts.map((p) => `${p[0].toFixed(1)},${p[1].toFixed(1)}`).join(' ');

/**
 * Start marker on the release point. Drawn pointing up (-y), centred on 0,0, about 60 map px
 * across. To use an aircraft image instead, return e.g.
 * <image href={aircraftUrl} x={-30} y={-30} width={60} height={60} />
 */
function ReleasePointMarker({ color = ROUTE }: { color?: string }) {
  return (
    <>
      <circle r={30} fill="#111" stroke={color} strokeWidth={6} />
      <path d="M0 -20 L14 14 L0 6 L-14 14 Z" fill="#fff" />
    </>
  );
}

function MapLabel({ x, y, z, fill, anchor = 'start', children }: { x: number; y: number; z: number; fill: string; anchor?: 'start' | 'middle' | 'end'; children: ReactNode }) {
  return (
    <text x={x} y={y} fill={fill} textAnchor={anchor} fontSize={20 * z} fontWeight={800} stroke="#000" strokeWidth={5 * z} strokeLinejoin="round" paintOrder="stroke" style={{ pointerEvents: 'none', fontFamily: 'system-ui, sans-serif' }}>
      {children}
    </text>
  );
}

export function TaxiClearanceMap({ route, preliminary = false, departureRunway = null, landingRunway = null }: { route: TaxiRoute; preliminary?: boolean; departureRunway?: string | null; landingRunway?: string | null }) {
  const routeColor = preliminary ? PRELIMINARY : ROUTE, routeDim = preliminary ? PRELIMINARY_DIM : ROUTE_DIM;
  const net = EKCH_TAXI_NETWORK;
  const svgRef = useRef<SVGSVGElement>(null);
  const drag = useRef<{ x: number; y: number; view: ViewBox } | null>(null);
  const arrowId = `taxi-arrow-${useId().replace(/[^a-zA-Z0-9]/g, '')}`;
  const all = routePoints(route);
  // The view frames where the flight is and where it is going (release point and departure runway, or
  // stand and landing runway) once, and keeps that zoom while the clearance is typed. It only refits
  // when one of those changes; the pilot can still zoom and pan.
  const anchor: Point | null = route.mode === 'departure'
    ? (route.releasePoint ? [route.releasePoint.x, route.releasePoint.y] : null)
    : (route.stand ? [route.stand.x, route.stand.y] : route.start);
  const focusRunway = route.mode === 'departure' ? departureRunway : landingRunway;
  const runwayPoint = anchor ? nearestOnRunway(focusRunway, anchor) : null;
  const fitKey = `${route.mode}|${anchor?.map((v) => v.toFixed(0)).join(',') ?? ''}|${findRunway(focusRunway)?.name ?? ''}`;
  const [view, setView] = useState<{ key: string; vb: ViewBox }>({ key: '', vb: FULL_VIEW });
  let vb = view.vb;
  if (view.key !== fitKey) {
    vb = fitView([...(anchor ? [anchor] : []), ...(runwayPoint ? [runwayPoint] : [])]);
    setView({ key: fitKey, vb });
  }

  useEffect(() => {
    const svg = svgRef.current;
    if (!svg) return;
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      const ctm = svg.getScreenCTM();
      if (!ctm) return;
      const m = new DOMPoint(event.clientX, event.clientY).matrixTransform(ctm.inverse());
      const factor = event.deltaY > 0 ? ZOOM_STEP : 1 / ZOOM_STEP;
      setView((current) => ({ ...current, vb: zoomView(current.vb, factor, [m.x, m.y]) }));
    };
    svg.addEventListener('wheel', onWheel, { passive: false });
    return () => svg.removeEventListener('wheel', onWheel);
  }, []);

  const zoom = (factor: number) => setView((current) => ({ ...current, vb: zoomView(current.vb, factor) }));

  const onPointerDown = (event: ReactPointerEvent<SVGSVGElement>) => {
    drag.current = { x: event.clientX, y: event.clientY, view: vb };
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const onPointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    const start = drag.current, svg = svgRef.current;
    if (!start || !svg) return;
    const scale = Math.max(start.view.w / svg.clientWidth, start.view.h / svg.clientHeight);
    setView((current) => ({ ...current, vb: { ...start.view, x: start.view.x - (event.clientX - start.x) * scale, y: start.view.y - (event.clientY - start.y) * scale } }));
  };
  const onPointerUp = () => { drag.current = null; };

  const z = Math.max(1, vb.w / 900, vb.h / 600);
  const a = arcs(all);
  const holdAt = route.holdAt ?? Number.POSITIVE_INFINITY;

  // Route pieces: solid up to the first hold short, dimmed after it.
  const pieces: Array<{ pts: Point[]; dim: boolean; dashed?: boolean; arrow?: boolean; past?: boolean }> = [];
  // Route points up to here belong to earlier clearances (grey).
  let pastUntil = -1;
  let offset = 0;
  for (const leg of route.legs) {
    // Off-map taxiways (e.g. B1-B3) connect at the map edge and are not drawn.
    if (leg.pts.length >= 2 && !leg.offMap) {
      const s0 = a[offset], s1 = a[offset + leg.pts.length - 1];
      if (s1 <= holdAt) pieces.push({ pts: leg.pts, dim: false, dashed: leg.connector, past: leg.past });
      else if (s0 >= holdAt) pieces.push({ pts: leg.pts, dim: true, dashed: leg.connector, past: leg.past });
      else {
        pieces.push({ pts: sliceArc(all, a, s0, holdAt), dim: false, dashed: leg.connector, past: leg.past });
        pieces.push({ pts: sliceArc(all, a, holdAt, s1), dim: true, dashed: leg.connector, past: leg.past });
      }
    }
    offset += leg.pts.length;
    if (leg.past) pastUntil = offset - 1;
  }
  if (route.stub && route.stub.pts.length >= 2 && !route.stub.offMap) {
    const holdOnStub = route.marks.some((mark) => mark.type === 'hold' && mark.g >= offset);
    if (holdOnStub) {
      if (holdAt > a[offset]) pieces.push({ pts: sliceArc(all, a, a[offset], holdAt), dim: false, past: route.stub.past });
    } else {
      pieces.push({ pts: route.stub.pts, dim: route.holdAt !== null, arrow: true, past: route.stub.past });
    }
  }


  // The aircraft sits where the active clearance starts: at the start of the route, or where the
  // grey (already cleared, before the last "/") part turns into the active route.
  const activeLeg = route.legs.find((leg) => !leg.past && leg.pts.length > 1);
  const first = (pastUntil >= 0 ? activeLeg?.pts ?? (route.stub && !route.stub.past ? route.stub.pts : null) : null)
    ?? route.legs.find((leg) => leg.pts.length > 1)?.pts ?? route.stub?.pts;
  const aircraft: Point | null = pastUntil >= 0 && first ? first[0] : route.start;
  const heading = first && first.length > 1 ? Math.atan2(first[Math.min(first.length - 1, 3)][1] - first[0][1], first[Math.min(first.length - 1, 3)][0] - first[0][0]) * 180 / Math.PI + 90 : 0;

  // Taxiway name signs (black on yellow), each placed along its own taxiway where it overlaps
  // neither another sign nor the aircraft, start or stand markers.
  // Departures: the assigned runway is the destination, marked in yellow like the stand for arrivals.
  const destination = route.mode === 'departure' ? findRunway(departureRunway, net) : null;
  const destinationEdges = destination ? net.edges.filter((edge) => edge.name === destination.name) : [];
  let destinationLabel: { x: number; y: number; anchor: 'start' | 'end' } | null = null;
  if (destinationEdges.length) {
    // Label the runway next to where the route gets closest to it (or its middle before there is a route).
    const target = route.stub?.pts.at(-1) ?? all.at(-1) ?? route.start;
    let best: { d: number; p: Point; u: Point } | null = null;
    for (const edge of destinationEdges) {
      for (let i = 0; i < edge.pts.length - 1; i++) {
        const p0 = edge.pts[i], p1 = edge.pts[i + 1], dx = p1[0] - p0[0], dy = p1[1] - p0[1], l2 = dx * dx + dy * dy;
        const t = target && l2 ? Math.max(0, Math.min(1, ((target[0] - p0[0]) * dx + (target[1] - p0[1]) * dy) / l2)) : 0.5;
        const p: Point = [p0[0] + t * dx, p0[1] + t * dy], d = target ? Math.hypot(target[0] - p[0], target[1] - p[1]) : 0;
        if (!best || d < best.d) best = { d, p, u: unit(p0, p1) };
      }
    }
    if (best) {
      const n: Point = [-best.u[1], best.u[0]], away = target && (target[0] - best.p[0]) * n[0] + (target[1] - best.p[1]) * n[1] > 0 ? -1 : 1;
      const x = best.p[0] + n[0] * 40 * z * away, y = best.p[1] + n[1] * 40 * z * away + 7 * z;
      destinationLabel = { x, y, anchor: n[0] * away < 0 ? 'end' : 'start' };
    }
  }

  const signFont = 12 * z, signH = 19 * z;
  type Box = { x0: number; y0: number; x1: number; y1: number };
  const overlaps = (p: Box, q: Box) => p.x0 < q.x1 && q.x0 < p.x1 && p.y0 < q.y1 && q.y0 < p.y1;
  const taken: Box[] = [];
  const keepClear = (p: Point | null | undefined, radius: number) => { if (p) taken.push({ x0: p[0] - radius, y0: p[1] - radius, x1: p[0] + radius, y1: p[1] + radius }); };
  keepClear(aircraft, 36 * AIRCRAFT_SCALE * z);
  if (aircraft !== route.start) keepClear(route.start, 14 * z);
  if (route.stand) keepClear([route.stand.x, route.stand.y], 12 * z);
  // Keep signs off the hold-short and crossing labels (same placement as where they are drawn below).
  const textBox = (x: number, y: number, text: string, anchor: 'start' | 'end'): Box => {
    const w = text.length * 0.62 * 20 * z;
    return { x0: anchor === 'start' ? x : x - w, y0: y - 18 * z, x1: anchor === 'start' ? x + w : x, y1: y + 6 * z };
  };
  for (const mark of route.marks) {
    const s = a[mark.g];
    if (s === undefined) continue;
    if (mark.type === 'hold') {
      const back = holdBack(mark), pos = atArc(all, a, s - back), u = unit(atArc(all, a, s - back - 12), pos), n: Point = [-u[1], u[0]], half = 36;
      keepClear(pos, half);
      taken.push(textBox(pos[0] - n[0] * (half + 14 * z) - u[0] * 10 * z, pos[1] - n[1] * (half + 14 * z) - u[1] * 10 * z + 7 * z, `HOLD SHORT ${mark.label}`, n[0] < 0 ? 'start' : 'end'));
    } else {
      const span = RUNWAY_HOLD_BACK + 10, seg = sliceArc(all, a, s - span, s + span), u = unit(seg[0], seg[seg.length - 1]), n: Point = [u[1], -u[0]];
      const co = CROSS_OFFSET * Math.max(1, z * 0.7), mid = atArc(seg, arcs(seg), arcs(seg)[seg.length - 1] / 2);
      keepClear(mid, co + 10);
      taken.push(textBox(mid[0] + n[0] * (co + 16 + 14 * z), mid[1] + n[1] * (co + 16 + 14 * z) + 7 * z, `CROSS ${mark.label}`, n[0] < 0 ? 'end' : 'start'));
    }
  }
  if (route.stand) taken.push(textBox(route.stand.x + 13 * z, route.stand.y - 10 * z, `STAND ${route.stand.name}`, 'start'));
  if (destination && destinationLabel) taken.push(textBox(destinationLabel.x, destinationLabel.y, `RWY ${destination.end}`, destinationLabel.anchor));

  const signs = [...route.legs.filter((leg) => !leg.connector), ...(route.stub ? [route.stub] : [])].filter((leg) => leg.pts.length >= 2 && !leg.offMap).map((leg) => {
    const la = arcs(leg.pts), length = la[la.length - 1], w = (leg.name.length * 0.64 + 0.9) * signFont;
    let chosen: Box | null = null, anchor: Point | null = null;
    search: for (const along of [0.5, 0.35, 0.65, 0.2, 0.8, 0.1, 0.9]) {
      for (const side of [1, -1]) {
        for (const gap of [30, 54]) {
          const s = length * along, m = atArc(leg.pts, la, s), u = unit(atArc(leg.pts, la, s - 10), atArc(leg.pts, la, s + 10));
          const cx = m[0] + u[1] * gap * z * side, cy = m[1] - u[0] * gap * z * side;
          const box = { x0: cx - w / 2, y0: cy - signH / 2, x1: cx + w / 2, y1: cy + signH / 2 };
          if (!chosen) { chosen = box; anchor = m; }
          if (!taken.some((t) => overlaps(t, box))) { chosen = box; anchor = m; break search; }
        }
      }
    }
    taken.push(chosen!);
    return { name: leg.name, past: leg.past, box: chosen!, anchor: anchor! };
  });

  const map = (
    <svg
      ref={svgRef}
      role="img"
      aria-label="Taxi route"
      viewBox={`${vb.x} ${vb.y} ${vb.w} ${vb.h}`}
      preserveAspectRatio="xMidYMid meet"
      className="h-full w-full cursor-grab touch-none select-none bg-[#0b0c0e]"
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
    >
      <defs>
        <marker id={arrowId} viewBox="0 0 10 10" refX={3} refY={5} markerWidth={3.2} markerHeight={3.2} orient="auto"><path d="M0 0 L10 5 L0 10 Z" fill={routeColor} /></marker>
        <marker id={`${arrowId}-dim`} viewBox="0 0 10 10" refX={3} refY={5} markerWidth={3.2} markerHeight={3.2} orient="auto"><path d="M0 0 L10 5 L0 10 Z" fill={routeDim} /></marker>
      </defs>
      <image href={chartUrl} width={net.chart.width} height={net.chart.height} opacity={0.6} />
      <g opacity={0.28}>
        {net.edges.map((edge, i) => <polyline key={i} points={polyline(edge.pts)} fill="none" stroke={edge.kind === 'runway' ? '#fff' : '#00e5ff'} strokeWidth={5} />)}
      </g>

      {destinationEdges.map((edge, i) => (
        <polyline key={`dest${i}`} points={polyline(edge.pts)} fill="none" stroke={RUNWAY_DESTINATION} strokeWidth={10} strokeOpacity={0.9} strokeLinecap="round" />
      ))}

      {pieces.map((piece, i) => (
        <g key={i}>
          <polyline points={polyline(piece.pts)} fill="none" stroke="#000" strokeOpacity={0.45} strokeWidth={ROUTE_WIDTH + 8} strokeLinecap="round" strokeLinejoin="round" />
          <polyline
            points={polyline(piece.pts)} fill="none" stroke={piece.past ? PAST : piece.dim ? routeDim : routeColor} strokeWidth={ROUTE_WIDTH} strokeLinecap="round" strokeLinejoin="round"
            strokeDasharray={piece.dashed ? '4 26' : undefined} markerEnd={piece.arrow ? `url(#${piece.dim ? `${arrowId}-dim` : arrowId})` : undefined}
          />
        </g>
      ))}

      {signs.map((sign, i) => (
        <g key={i} style={{ pointerEvents: 'none' }}>
          {/* Leader line from the sign to the point on its taxiway; the sign covers the inner end. */}
          <line x1={sign.anchor[0]} y1={sign.anchor[1]} x2={(sign.box.x0 + sign.box.x1) / 2} y2={(sign.box.y0 + sign.box.y1) / 2} stroke="#000" strokeWidth={2 * z} strokeLinecap="round" />
          <rect x={sign.box.x0} y={sign.box.y0} width={sign.box.x1 - sign.box.x0} height={sign.box.y1 - sign.box.y0} rx={3 * z} fill={sign.past ? PAST : SIGN} stroke="#000" strokeWidth={2 * z} />
          <text x={(sign.box.x0 + sign.box.x1) / 2} y={(sign.box.y0 + sign.box.y1) / 2 + signFont * 0.36} textAnchor="middle" fontSize={signFont} fontWeight={800} fill={sign.past ? '#3b4250' : '#000'} style={{ fontFamily: 'system-ui, sans-serif' }}>{sign.name}</text>
        </g>
      ))}

      {route.marks.map((mark, i) => {
        const s = a[mark.g];
        if (s === undefined) return null;
        if (mark.type === 'hold') {
          const back = holdBack(mark), pos = atArc(all, a, s - back), u = unit(atArc(all, a, s - back - 12), pos), n: Point = [-u[1], u[0]], half = 36;
          const ends = { x1: pos[0] - n[0] * half, y1: pos[1] - n[1] * half, x2: pos[0] + n[0] * half, y2: pos[1] + n[1] * half };
          return (
            <g key={i}>
              <line {...ends} stroke="#000" strokeWidth={18} strokeLinecap="round" />
              <line {...ends} stroke={HOLD} strokeWidth={12} strokeLinecap="round" />
              <MapLabel x={pos[0] - n[0] * (half + 14 * z) - u[0] * 10 * z} y={pos[1] - n[1] * (half + 14 * z) - u[1] * 10 * z + 7 * z} z={z} fill="#ff5a5a" anchor={n[0] < 0 ? 'start' : 'end'}>
                HOLD SHORT {mark.label}
              </MapLabel>
            </g>
          );
        }
        // Cleared to cross: a green rail on each side of the route across the runway.
        const span = RUNWAY_HOLD_BACK + 10, seg = sliceArc(all, a, s - span, s + span), u = unit(seg[0], seg[seg.length - 1]), n: Point = [u[1], -u[0]];
        const co = CROSS_OFFSET * Math.max(1, z * 0.7);
        const rails = [1, -1].map((side) => seg.map((p): Point => [p[0] + n[0] * co * side, p[1] + n[1] * co * side]));
        const ra = arcs(rails[0]), mid = atArc(rails[0], ra, ra[ra.length - 1] / 2);
        return (
          <g key={i}>
            {rails.map((rail, j) => (
              <g key={j}>
                <polyline points={polyline(rail)} fill="none" stroke="#000" strokeOpacity={0.6} strokeWidth={12} strokeLinecap="round" />
                <polyline points={polyline(rail)} fill="none" stroke={mark.g <= pastUntil ? PAST : CROSS} strokeWidth={7} strokeLinecap="round" />
                {[rail[0], rail[rail.length - 1]].map((p, k) => <circle key={k} cx={p[0]} cy={p[1]} r={6} fill={mark.g <= pastUntil ? PAST : CROSS} stroke="#000" strokeWidth={2} />)}
              </g>
            ))}
            <MapLabel x={mid[0] + n[0] * (16 + 14 * z)} y={mid[1] + n[1] * (16 + 14 * z) + 7 * z} z={z} fill={mark.g <= pastUntil ? PAST : CROSS} anchor={n[0] < 0 ? 'end' : 'start'}>CROSS {mark.label}</MapLabel>
          </g>
        );
      })}

      {route.stand && (
        <g>
          {route.standReached && route.standEntry && (
            <line x1={route.standEntry[0]} y1={route.standEntry[1]} x2={route.stand.x} y2={route.stand.y} stroke={routeColor} strokeWidth={ROUTE_WIDTH * 0.6} strokeDasharray="10 12" strokeLinecap="round" />
          )}
          <rect x={route.stand.x - 8 * z} y={route.stand.y - 8 * z} width={16 * z} height={16 * z} rx={3 * z} fill={route.standReached ? routeColor : '#ffd84a'} stroke="#000" strokeWidth={2 * z} />
          <MapLabel x={route.stand.x + 13 * z} y={route.stand.y - 10 * z} z={z} fill="#ffe27a">STAND {route.stand.name}</MapLabel>
        </g>
      )}

      {destination && destinationLabel && (
        <MapLabel x={destinationLabel.x} y={destinationLabel.y} z={z} fill="#ffe27a" anchor={destinationLabel.anchor}>RWY {destination.end}</MapLabel>
      )}

      {route.start && route.startLabel && aircraft !== route.start && (
        // The route moved on (onward clearance): the original start keeps a small marker.
        <g>
          <circle cx={route.start[0]} cy={route.start[1]} r={10 * z} fill={PAST} stroke="#000" strokeWidth={3 * z} />
        </g>
      )}
      {aircraft && route.startLabel && (
        <g>
          <g transform={`translate(${aircraft[0]} ${aircraft[1]}) rotate(${heading}) scale(${z * AIRCRAFT_SCALE})`}><ReleasePointMarker color={routeColor} /></g>
        </g>
      )}
    </svg>
  );

  const zoomButtonClass = 'grid size-11 place-items-center rounded border border-[#58708a] bg-[#0a1d31] text-2xl font-bold text-white hover:bg-[#1A475F] focus-visible:outline-2 focus-visible:outline-[#8ed7ec]';
  return (
    <div className="relative h-full w-full">
      {map}
      <div role="group" aria-label="Taxi map zoom" className="absolute top-2 right-2 flex gap-1">
        <button type="button" aria-label="Zoom in on taxi map" onClick={() => zoom(1 / ZOOM_STEP)} className={zoomButtonClass}>+</button>
        <button type="button" aria-label="Zoom out on taxi map" onClick={() => zoom(ZOOM_STEP)} className={zoomButtonClass}>−</button>
      </div>
    </div>
  );
}

const chipClass: Record<string, string> = {
  rp: 'border-[#ff2bd6] bg-[#2a1530] text-[#ffb3f0]',
  twy: 'border-[#7a3a72] bg-[#25182b] text-white',
  rwy: 'border-[#8ea2c8] bg-[#1d2230] text-[#dfe7ff]',
  hold: 'border-[#ff3b3b] bg-[#2b1414] text-[#ffb0b0]',
  cross: 'border-[#2fe07a] bg-[#10291b] text-[#9ff5c2]',
  bad: 'border-[#ff3b3b] bg-[#3a1c1c] text-[#ffb0b0] underline decoration-wavy',
  stand: 'border-[#ffd84a] bg-[#2b2610] text-[#ffe27a]',
  past: 'border-[#8b93a1] bg-[#2a2f38] text-[#d3d8df]',
};
const messageClass: Record<string, string> = {
  ok: 'border-[#2fe07a] bg-[#10291b] text-[#bdf5d3]',
  info: 'border-[#5b8cff] bg-[#13203a] text-[#c9dbff]',
  warn: 'border-[#ffb020] bg-[#2c2410] text-[#ffe0a3]',
  err: 'border-[#ff3b3b] bg-[#2b1414] text-[#ffc4c4]',
};
const chip = 'shrink-0 rounded border px-1 py-px font-mono text-[11px] leading-4 font-bold';
const inputClass ='w-full rounded-md border border-[#58708a] bg-[#0a1d31] px-3 py-2 font-mono text-lg font-bold tracking-wider text-white uppercase outline-none focus:border-[#ff2bd6] focus:shadow-[0_0_0_3px_#ff2bd633]';

export function TaxiClearanceControls({ state }: { state: TaxiClearanceState }) {
  const { route } = state;
  const messages = state.preliminary ? route.messages.filter(({ level }) => level === 'warn' || level === 'err') : route.messages;
  // A preliminary route is not the pilot's clearance: do not spell it out.
  const items = state.preliminary ? [] : route.parsed.items;
  const startChip = route.mode === 'arrival' ? (state.landingRunway ? `RWY ${state.landingRunway}` : null) : route.releasePoint?.name ?? null;

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-4 px-5 py-4">
      {state.mode === 'arrival' ? (
        <div className="grid grid-cols-2 gap-3">
          <div>
            <div className="flex items-center justify-between gap-2">
              <label htmlFor="efb-taxi-landing-runway" className="text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">LDG RWY</label>
              {state.fromFlight.runway && <span className="rounded bg-[#1A475F] px-1.5 py-0.5 text-[10px] font-bold text-[#a9ecff]">FLIGHT</span>}
            </div>
            <input id="efb-taxi-landing-runway" value={state.landingRunway} onChange={(event) => state.setLandingRunway(event.target.value)} placeholder="22L" autoComplete="off" spellCheck={false} className={`${inputClass} mt-2`} />
          </div>
          <div>
            <div className="flex items-center justify-between gap-2">
              <label htmlFor="efb-taxi-stand" className="text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">STAND</label>
              {state.fromFlight.stand && <span className="rounded bg-[#1A475F] px-1.5 py-0.5 text-[10px] font-bold text-[#a9ecff]">FLIGHT</span>}
            </div>
            <input id="efb-taxi-stand" value={state.arrivalStand} onChange={(event) => state.setArrivalStand(event.target.value)} placeholder="F98" autoComplete="off" spellCheck={false} className={`${inputClass} mt-2`} />
          </div>
        </div>
      ) : (
      <div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <div className="flex items-center justify-between gap-2">
              <label htmlFor="efb-taxi-release-point" className="text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">RELEASE POINT</label>
              {state.source === 'atc' && <span className="rounded bg-[#1A475F] px-1.5 py-0.5 text-[10px] font-bold text-[#a9ecff]">ATC</span>}
              {state.source === 'stand' && <span className="rounded bg-[#1A475F] px-1.5 py-0.5 text-[10px] font-bold text-[#a9ecff]">STAND</span>}
              {state.source === 'pilot' && <span className="rounded bg-[#254967] px-1.5 py-0.5 text-[10px] font-bold text-white">ENTERED</span>}
            </div>
            <input id="efb-taxi-release-point" value={state.releasePoint} onChange={(event) => state.setReleasePoint(event.target.value)} placeholder="Z3" autoComplete="off" spellCheck={false} className={`${inputClass} mt-2`} />
          </div>
          <div>
            <div className="flex items-center justify-between gap-2">
              <label htmlFor="efb-taxi-departure-runway" className="text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">DEP RWY</label>
              {state.departureRunwaySource === 'flight' && <span className="rounded bg-[#1A475F] px-1.5 py-0.5 text-[10px] font-bold text-[#a9ecff]">FLIGHT</span>}
              {state.departureRunwaySource === 'clearance' && <span className="rounded bg-[#1A475F] px-1.5 py-0.5 text-[10px] font-bold text-[#a9ecff]">CLEARANCE</span>}
              {state.departureRunwaySource === 'pilot' && <span className="rounded bg-[#254967] px-1.5 py-0.5 text-[10px] font-bold text-white">ENTERED</span>}
            </div>
            {/* Shows the runway implied by the clearance as a placeholder until the flight or the pilot sets one. */}
            <input id="efb-taxi-departure-runway" value={state.landingRunway} onChange={(event) => state.setLandingRunway(event.target.value)} placeholder={state.departureRunway ?? '22R'} autoComplete="off" spellCheck={false} className={`${inputClass} mt-2`} />
          </div>
        </div>
        {state.suggestion && (
          <div role="status" className="mt-2 flex items-center justify-between gap-2 border-l-[3px] border-[#5b8cff] bg-[#13203a] px-3 py-2 text-sm text-[#c9dbff]">
            <span>{state.suggestion.from === 'stand' ? `Default for stand ${state.suggestion.stand}` : 'ATC assigned'} <b>{state.suggestion.point}</b></span>
            <button type="button" onClick={state.acceptSuggestion} className="border border-[#8ed7ec] bg-[#1A475F] px-2 py-1 text-xs font-bold text-white">USE {state.suggestion.point}</button>
          </div>
        )}
        {state.atcUnknown && <p role="alert" className="mt-2 mb-0 text-sm text-[#ffe0a3]">ATC assigned {state.atcUnknown}, which is not on the taxi map. Enter the nearest release point.</p>}
      </div>
      )}

      <div>
        <div className="flex items-center justify-between gap-2">
          <label htmlFor="efb-taxi-clearance" className="text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">TAXI CLEARANCE</label>
          {state.preliminary && <span className="rounded bg-[#0e3a44] px-2 py-0.5 text-[11px] font-bold text-[#67e8f9]">PRELIMINARY</span>}
        </div>
        {state.mode === 'arrival' && (
          <p className="mt-1 mb-0 text-xs text-[#c7d9e5]">
            Start with the taxiway you vacate onto.
          </p>
        )}
        <input id="efb-taxi-clearance" value={state.clearance} onChange={(event) => state.setClearance(event.target.value)} maxLength={MAX_CLEARANCE_LENGTH} placeholder="WRITE HERE" autoComplete="off" spellCheck={false} className={`${inputClass} mt-2`} />
        <div aria-label="Clearance parts" className="efb-scroll mt-2 flex min-h-6 flex-nowrap items-center gap-0.5 overflow-x-auto whitespace-nowrap">
          {startChip && <span className={`${chip} ${chipClass.rp}`}>{startChip}</span>}
          {items.map((item, i) => (
            <span key={i} className="flex shrink-0 items-center gap-0.5">
              {i > 0 && item.segment !== items[i - 1].segment
                ? <span className="px-0.5 text-[12px] font-bold text-[#c7d9e5]">/</span>
                : (i > 0 || startChip) && <span className="text-[10px] text-[#7c9bb3]">›</span>}
              <span className={`${chip} ${item.segment < route.parsed.activeSegment ? chipClass.past : chipClass[item.type]}`}>
                {item.type === 'hold' ? 'HS ' : item.type === 'cross' ? 'X ' : item.type === 'rwy' ? 'RWY ' : ''}{itemLabel(item)}
              </span>
            </span>
          ))}
          {route.parsed.bad && <span className={`${chip} ${chipClass.bad}`}>{route.parsed.bad}</span>}
          {route.stand && (
            <span className="flex shrink-0 items-center gap-0.5">
              <span className="text-[10px] text-[#7c9bb3]">›</span>
              <span className={`${chip} ${route.standReached ? chipClass.rp : chipClass.stand}`}>STAND {route.stand.name}</span>
            </span>
          )}
        </div>
      </div>

      <div className="grid gap-2" aria-live="polite">
        {state.preliminary && <div className="border-l-[3px] border-[#22d3ee] bg-[#0b2a33] px-3 py-2 text-sm text-[#a5f3fc]">Preliminary route in cyan</div>}
        {messages.map((message, i) => <div key={i} className={`border-l-[3px] px-3 py-2 text-sm ${messageClass[message.level]}`}>{message.text}</div>)}
      </div>
    </div>
  );
}

export function TaxiCommonRoutes({ state }: { state: TaxiClearanceState }) {
  const runway = state.departureRunway?.trim();
  const rwy = runway && runway !== 'NIL' ? runway : '';
  const routes = rwy ? commonRoutesFor(rwy, state.releasePoint) : [];
  const byHoldingPoint = new Map<string, CommonRoute[]>();
  for (const route of routes) byHoldingPoint.set(route.holdingPoint, [...(byHoldingPoint.get(route.holdingPoint) ?? []), route]);

  return (
    <section aria-labelledby="efb-taxi-common-routes" className="border-t border-[#58708a] px-5 py-4">
      <div className="flex items-baseline justify-between gap-3">
        <h3 id="efb-taxi-common-routes" className="m-0 text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">COMMON ROUTES</h3>
        <span className="shrink-0 text-xs text-[#c7d9e5]">{rwy ? `Runway ${rwy}` : 'No runway assigned'}</span>
      </div>
      {!routes.length && <p className="mt-2 mb-0 text-sm text-[#c7d9e5]">{rwy ? `No common routes for runway ${rwy} yet.` : 'Common routes appear once a departure runway is assigned.'}</p>}
      {[...byHoldingPoint].map(([holdingPoint, list]) => (
        <div key={holdingPoint} className="mt-3">
          <p className="m-0 text-xs font-bold text-white">Holding point {holdingPoint}</p>
          <div className="mt-1.5 grid gap-1.5">
            {list.map((route) => {
              const selected = route.clearance.toUpperCase() === state.clearance.trim().toUpperCase();
              return (
                <button key={route.clearance} type="button" aria-pressed={selected} onClick={() => state.setClearance(route.clearance)} className={`border p-2 text-left font-mono text-sm font-bold transition-colors ${selected ? 'border-[#ff2bd6] bg-[#2a1530] text-white' : 'border-[#58708a] bg-[#0a1d31] text-white hover:border-[#8ed7ec]'}`}>
                  {route.clearance}
                  {route.note && <span className="mt-0.5 block font-sans text-xs font-normal text-[#c7d9e5]">{route.note}</span>}
                </button>
              );
            })}
          </div>
        </div>
      ))}
    </section>
  );
}

// Rows of the "How to write it" table: what you type, what it means, an example.
const WRITING_RULES: Array<[string, string, string]> = [
  ['KZA', 'Taxiways in order', 'KZA4'],
  ['.', 'Hold short of', '.22R'],
  ['x', 'Cross runway', 'x30'],
  ['30', 'Taxi ON or VIA a Runway', 'KZ30'],
  ['/', 'Onward clearance (earlier part turns grey)', 'KZ.A/A4'],
];

export function TaxiClearanceHelp() {
  return (
    <div className="border-t border-[#58708a] px-5 py-4">
      <p className="m-0 text-[11px] font-bold tracking-[0.16em] text-[#8ed7ec]">HOW TO WRITE IT</p>
      <table className="mt-3 w-full border-collapse text-left text-white">
        <thead>
          <tr className="text-[11px] tracking-[0.12em] text-[#8ed7ec]">
            <th className="border-b border-[#58708a] pb-1.5 font-bold">TYPE</th>
            <th className="border-b border-[#58708a] pb-1.5 font-bold">MEANS</th>
            <th className="border-b border-[#58708a] pb-1.5 font-bold">E.G.</th>
          </tr>
        </thead>
        <tbody>
          {WRITING_RULES.map(([type, means, example]) => (
            <tr key={type} className="border-b border-[#1f3a55] last:border-b-0">
              <td className="py-2 pr-3 align-middle font-mono text-[clamp(18px,2.4vh,24px)] font-bold text-[#8ed7ec]">{type}</td>
              <td className="py-2 pr-3 align-middle text-[clamp(14px,1.9vh,17px)] leading-snug">{means}</td>
              <td className="py-2 align-middle font-mono text-[clamp(14px,1.9vh,17px)] font-bold whitespace-nowrap text-[#c7d9e5]">{example}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="mt-2 mb-0 text-[clamp(12px,1.6vh,14px)] text-[#c7d9e5]">Spaces are optional.</p>
    </div>
  );
}
