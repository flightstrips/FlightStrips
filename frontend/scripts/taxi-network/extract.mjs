// Extract taxiway centrelines from a Figma-exported SVG, compute junctions, emit graph + debug page.
import fs from "node:fs";
import path from "node:path";

const [, , svgPath, outDir, namesPath] = process.argv;
const NAMES = namesPath ? JSON.parse(fs.readFileSync(namesPath, "utf8")) : { names: {}, runways: {}, splits: [] };
if (NAMES.splits?.length) throw new Error("line splits are not supported yet");
const svg = fs.readFileSync(svgPath, "utf8");
fs.mkdirSync(outDir, { recursive: true });

// Background image
const img = svg.match(/<image[^>]*xlink:href="data:image\/(\w+);base64,([^"]+)"/);
if (img) fs.writeFileSync(path.join(outDir, `chart.${img[1]}`), Buffer.from(img[2], "base64"));

// Stroked paths only (skip the filled frame)
const pathTags = [...svg.matchAll(/<path\b[^>]*>/g)].map((m) => m[0]);
const attr = (t, n) => (t.match(new RegExp(`\\b${n}="([^"]*)"`)) || [])[1];
const lines = [];
for (const t of pathTags) {
  if (!attr(t, "stroke")) continue;
  const id = attr(t, "id") || null;
  lines.push({ idx: lines.length + 1, id, pts: flatten(attr(t, "d")) });
}

// Overrides: extend a dangling line end straight along its last segment until it hits another line.
// Keyed by line number (#n in the debug view); "start" / "end" = which end of the drawn path.
const EXTEND = [
  { line: 14, end: "start", note: "K2 onto RWY 12/30" },
  { line: 15, end: "start", note: "K3 onto RWY 12/30" },
  { line: 8, end: "start", note: "G2 onto RWY 12/30 (east)" },
  { line: 31, end: "start", note: "A1 onto RWY 04L/22R" },
  { line: 32, end: "start", note: "A2 onto RWY 04L/22R" },
  { line: 33, end: "start", note: "A3 onto RWY 04L/22R", max: 260 },
  { line: 34, end: "start", note: "A4 onto RWY 04L/22R", max: 260 },
  { line: 35, end: "start", note: "A5 onto RWY 04L/22R", max: 260 },
  { line: 36, end: "start", note: "E1 onto RWY 04L/22R" },
  { line: 3, end: "end", note: "C onto RWY 04R/22L" },
  { line: 11, end: "start", note: "I onto RWY 04R/22L" },
  { line: 25, end: "end", note: "V2 onto RWY 04R/22L" },
  { line: 26, end: "end", note: "V1 onto RWY 04R/22L" },
];
const MAX_EXTEND = 150; // px
for (const ex of EXTEND) {
  const ln = lines.find((l) => l.idx === ex.line);
  const P = ln.pts;
  const [tip, prev] = ex.end === "start" ? [P[0], P[1]] : [P[P.length - 1], P[P.length - 2]];
  const len = Math.hypot(tip[0] - prev[0], tip[1] - prev[1]);
  const dir = [(tip[0] - prev[0]) / len, (tip[1] - prev[1]) / len];
  const maxLen = ex.max ?? MAX_EXTEND;
  const far = [tip[0] + dir[0] * maxLen, tip[1] + dir[1] * maxLen];
  let hit = null;
  for (const other of lines) {
    if (other === ln) continue;
    for (let k = 0; k < other.pts.length - 1; k++) {
      const x = segX(tip, far, other.pts[k], other.pts[k + 1]);
      if (x && (!hit || x.t < hit.t)) hit = { ...x, line: other.idx };
    }
  }
  if (!hit) { console.warn(`extend #${ex.line} (${ex.note}): no line within ${maxLen}px`); continue; }
  // end exactly on the other line; endpoint snapping turns it into a junction
  if (ex.end === "start") P.unshift(hit.pt); else P.push(hit.pt);
  console.log(`extend #${ex.line} (${ex.note}): +${Math.round(hit.t * maxLen)}px to line #${hit.line}`);
}

function flatten(d) {
  const toks = d.match(/[MLHVCZmlhvcz]|-?\d*\.?\d+(?:e-?\d+)?/g);
  const pts = [];
  let i = 0, cmd = "", x = 0, y = 0;
  const num = () => parseFloat(toks[i++]);
  while (i < toks.length) {
    if (/[A-Za-z]/.test(toks[i])) cmd = toks[i++];
    const rel = cmd === cmd.toLowerCase();
    const C = cmd.toUpperCase();
    if (C === "M" || C === "L") { const nx = num(), ny = num(); x = rel ? x + nx : nx; y = rel ? y + ny : ny; pts.push([x, y]); }
    else if (C === "H") { const n = num(); x = rel ? x + n : n; pts.push([x, y]); }
    else if (C === "V") { const n = num(); y = rel ? y + n : n; pts.push([x, y]); }
    else if (C === "C") {
      const a = [num(), num(), num(), num(), num(), num()];
      const [x1, y1, x2, y2, ex, ey] = rel ? a.map((v, k) => v + (k % 2 ? y : x)) : a;
      for (let s = 1; s <= 12; s++) {
        const u = s / 12, v = 1 - u;
        pts.push([v*v*v*x + 3*v*v*u*x1 + 3*v*u*u*x2 + u*u*u*ex, v*v*v*y + 3*v*v*u*y1 + 3*v*u*u*y2 + u*u*u*ey]);
      }
      x = ex; y = ey;
    } else if (C === "Z") { /* ignore */ }
    else throw new Error("Unsupported command " + cmd);
  }
  return pts;
}

// Geometry helpers
function segX(p, q, r, s) {
  const d = (q[0]-p[0])*(s[1]-r[1]) - (q[1]-p[1])*(s[0]-r[0]);
  if (Math.abs(d) < 1e-9) return null;
  const t = ((r[0]-p[0])*(s[1]-r[1]) - (r[1]-p[1])*(s[0]-r[0])) / d;
  const u = ((r[0]-p[0])*(q[1]-p[1]) - (r[1]-p[1])*(q[0]-p[0])) / d;
  if (t < 0 || t > 1 || u < 0 || u > 1) return null;
  return { pt: [p[0] + t*(q[0]-p[0]), p[1] + t*(q[1]-p[1])], t, u };
}
function closest(pt, a, b) {
  const dx = b[0]-a[0], dy = b[1]-a[1], L = dx*dx + dy*dy;
  const t = L ? Math.max(0, Math.min(1, ((pt[0]-a[0])*dx + (pt[1]-a[1])*dy) / L)) : 0;
  const c = [a[0] + t*dx, a[1] + t*dy];
  return { pt: c, t, dist: Math.hypot(pt[0]-c[0], pt[1]-c[1]) };
}

const SNAP = 15; // px: endpoint within this distance of another line counts as connected
// cuts[lineIdx] = [{pos (segment index + t), junction}]
const cuts = lines.map(() => []);
const junctions = [];
function addJunction(pt, a, posA, b, posB) {
  let j = junctions.find((j) => Math.hypot(j.pt[0]-pt[0], j.pt[1]-pt[1]) < SNAP);
  if (!j) { j = { id: junctions.length + 1, pt, lines: new Set() }; junctions.push(j); }
  j.lines.add(a); j.lines.add(b);
  cuts[a].push({ pos: posA, j: j.id });
  cuts[b].push({ pos: posB, j: j.id });
}

for (let a = 0; a < lines.length; a++) for (let b = a + 1; b < lines.length; b++) {
  const A = lines[a].pts, B = lines[b].pts;
  for (let i = 0; i < A.length - 1; i++) for (let k = 0; k < B.length - 1; k++) {
    const x = segX(A[i], A[i+1], B[k], B[k+1]);
    if (x) addJunction(x.pt, a, i + x.t, b, k + x.u);
  }
  // endpoint snapping both ways
  for (const [p, q, P, Q] of [[a, b, A, B], [b, a, B, A]]) {
    for (const end of [0, P.length - 1]) {
      let best = null;
      for (let k = 0; k < Q.length - 1; k++) {
        const c = closest(P[end], Q[k], Q[k+1]);
        if (!best || c.dist < best.dist) best = { ...c, k };
      }
      if (best && best.dist < SNAP) {
        const posP = end === 0 ? 0 : P.length - 1;
        addJunction(best.pt, p, posP, q, best.k + best.t);
      }
    }
  }
}

// Endpoint nodes for dangling ends, then split each line into edges between successive nodes
const nodes = junctions.map((j) => ({ id: `J${j.id}`, x: +j.pt[0].toFixed(1), y: +j.pt[1].toFixed(1), lines: [...j.lines].map((l) => lines[l].idx) }));
const edges = [];
lines.forEach((ln, li) => {
  const c = [...cuts[li]];
  const last = ln.pts.length - 1;
  const hasNear = (pos) => c.some((x) => Math.abs(x.pos - pos) < 0.05);
  for (const pos of [0, last]) if (!hasNear(pos)) {
    const id = `E${ln.idx}${pos === 0 ? "a" : "b"}`;
    nodes.push({ id, x: ln.pts[pos][0], y: ln.pts[pos][1], lines: [ln.idx], end: true });
    c.push({ pos, j: id });
  }
  c.sort((p, q) => p.pos - q.pos);
  const nodeId = (x) => (typeof x.j === "number" ? `J${x.j}` : x.j);
  const at = (pos) => { const i = Math.min(Math.floor(pos), last - 1), t = pos - i; const p = ln.pts[i], q = ln.pts[i+1]; return [p[0] + t*(q[0]-p[0]), p[1] + t*(q[1]-p[1])]; };
  for (let n = 0; n < c.length - 1; n++) {
    const from = c[n], to = c[n+1];
    if (nodeId(from) === nodeId(to)) continue;
    const pts = [at(from.pos)];
    for (let i = Math.ceil(from.pos); i < to.pos; i++) if (i > from.pos) pts.push(ln.pts[i]);
    pts.push(at(to.pos));
    let len = 0; for (let i = 1; i < pts.length; i++) len += Math.hypot(pts[i][0]-pts[i-1][0], pts[i][1]-pts[i-1][1]);
    edges.push({ line: ln.idx, name: ln.id, from: nodeId(from), to: nodeId(to), length: +len.toFixed(1), pts: pts.map((p) => [+p[0].toFixed(1), +p[1].toFixed(1)]) });
  }
});

for (const ln of lines) {
  const rwy = NAMES.runways?.[ln.idx];
  ln.kind = rwy ? "runway" : "taxiway";
  ln.name = rwy ? rwy.name : NAMES.names?.[ln.idx] ?? null;
  if (rwy) ln.ends = rwy.ends;
}
for (const e of edges) { const ln = lines.find((l) => l.idx === e.line); e.name = ln.name; e.kind = ln.kind; }
for (const n of nodes) n.names = [...new Set(n.lines.map((i) => lines.find((l) => l.idx === i).name))];
const graph = { width: 4096, height: 2301, lines: lines.map((l) => ({ idx: l.idx, name: l.name, kind: l.kind, ...(l.ends ? { ends: l.ends } : {}) })), nodes, edges };
fs.writeFileSync(path.join(outDir, "graph.json"), JSON.stringify(graph, null, 1));
console.log(`lines=${lines.length} junctions=${junctions.length} nodes=${nodes.length} edges=${edges.length}`);
