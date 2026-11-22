// Builds the EFB taxi network data file from the hand-drawn taxi map.
//
// Pipeline (run from frontend/):
//   1. node scripts/taxi-network/extract.mjs "<Taxi Map.svg>" <tmpDir> scripts/taxi-network/taxiway-names.json
//        -> <tmpDir>/graph.json (lines, junctions, edges) and <tmpDir>/chart.png (the embedded chart)
//   2. node scripts/taxi-network/shrink-png.mjs <tmpDir>/chart.png src/assets/efb/taxi/ekch-taxi-chart.png 4
//   3. node scripts/taxi-network/bundle.mjs <tmpDir>/graph.json
//        -> src/components/efb/taxi/ekch-taxi-network.json
//
// taxiway-names.json maps drawn line numbers to taxiway names, runway lines to their ends, and
// release-points.json holds the release point positions placed on the same map, and
// off-map-taxiways.json lists taxiways that continue past the map edge (e.g. B1-B3 to 04R).
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const [, , graphPath, outPath = path.join(here, "../../src/components/efb/taxi/ekch-taxi-network.json")] = process.argv;
if (!graphPath) throw new Error("usage: node bundle.mjs <graph.json> [out.json]");

const graph = JSON.parse(fs.readFileSync(graphPath, "utf8"));
const releasePoints = JSON.parse(fs.readFileSync(path.join(here, "release-points.json"), "utf8")).releasePoints;
const unnamed = graph.lines.filter((line) => !line.name);
if (unnamed.length) throw new Error(`unnamed lines: ${unnamed.map((line) => `#${line.idx}`).join(", ")}`);

const network = {
  airport: "EKCH",
  width: graph.width,
  height: graph.height,
  chart: { width: 4096, height: 2286 },
  lines: graph.lines.map(({ name, kind, ends }) => (ends ? { name, kind, ends } : { name, kind })),
  nodes: graph.nodes.map(({ id, x, y, names }) => ({ id, x, y, names })),
  edges: graph.edges.map(({ from, to, name, kind, pts }) => ({ from, to, name, kind, pts })),
  releasePoints: releasePoints.map(({ name, x, y }) => ({ name, x, y })),
};

// Stand positions, and where each joins the taxi network (arrival routes end there). A stand can join
// several taxiways: the one it was placed next to, plus any listed in stand-extra-joins.json.
const extraJoins = JSON.parse(fs.readFileSync(path.join(here, "stand-extra-joins.json"), "utf8")).joins;
const nearestOn = (taxiway, x, y) => {
  let best = null;
  for (const e of graph.edges) {
    if (e.name !== taxiway) continue;
    for (let i = 0; i < e.pts.length - 1; i++) {
      const [a, b] = [e.pts[i], e.pts[i + 1]], dx = b[0] - a[0], dy = b[1] - a[1], l2 = dx * dx + dy * dy;
      const t = l2 ? Math.max(0, Math.min(1, ((x - a[0]) * dx + (y - a[1]) * dy) / l2)) : 0;
      const c = [a[0] + t * dx, a[1] + t * dy], d = Math.hypot(x - c[0], y - c[1]);
      if (!best || d < best.d) best = { d, x: +c[0].toFixed(1), y: +c[1].toFixed(1) };
    }
  }
  if (!best) throw new Error(`unknown taxiway ${taxiway}`);
  return { x: best.x, y: best.y, taxiway };
};
const stands = JSON.parse(fs.readFileSync(path.join(here, "stands.json"), "utf8")).stands;
for (const name of Object.keys(extraJoins)) if (!stands.some((s) => s.name === name)) throw new Error(`unknown stand ${name}`);
network.stands = stands.map(({ name, x, y, entry }) => ({
  name, x, y,
  entries: [entry, ...(extraJoins[name] ?? []).filter((t) => t !== entry.taxiway).map((t) => nearestOn(t, x, y))],
}));

// Taxiways beyond the map edge: they connect where their parent taxiway leaves the map and lead to
// their runway. They have no drawn geometry: a zero-length edge from the parent's map-edge end to a
// node that belongs to the runway, so clearances and hold shorts resolve at the map edge.
const offMapConfig = JSON.parse(fs.readFileSync(path.join(here, "off-map-taxiways.json"), "utf8"));
const border = (n) => Math.min(n.x, n.y, graph.width - n.x, graph.height - n.y);
const mapEdgeEnd = (taxiway) => {
  const end = graph.nodes.filter((n) => n.end && n.names.includes(taxiway)).sort((a, b) => border(a) - border(b))[0];
  if (!end || border(end) > 30) throw new Error(`${taxiway} does not leave the map`);
  return end;
};
// A taxiway that reaches a runway beyond the map edge. Only used as an arrival exit (vacating the
// runway onto that taxiway at the map edge); departures still need an explicit way onto the runway.
network.runwayLinks = (offMapConfig.runwayLinks ?? []).map((link) => {
  if (!network.lines.some((line) => line.name === link.runway)) throw new Error(`unknown runway ${link.runway}`);
  return { taxiway: link.taxiway, runway: link.runway, node: mapEdgeEnd(link.taxiway).id };
});
// Linked taxiways marked 'connect' lead onto their runway beyond the edge: a zero-length edge of that
// taxiway to a node on the runway, so a route along it can hold short there.
for (const link of offMapConfig.runwayLinks ?? []) {
  if (!link.connect) continue;
  const end = mapEdgeEnd(link.taxiway), id = `OFF_${link.taxiway}_RWY`;
  network.nodes.push({ id, x: end.x, y: end.y, names: [link.taxiway, link.runway] });
  network.edges.push({ from: end.id, to: id, name: link.taxiway, kind: 'taxiway', pts: [[end.x, end.y], [end.x, end.y]] });
}
for (const taxiway of offMapConfig.taxiways) {
  const end = mapEdgeEnd(taxiway.from);
  if (!network.lines.some((line) => line.name === taxiway.runway)) throw new Error(`unknown runway ${taxiway.runway}`);
  const id = `OFF_${taxiway.name}`;
  network.lines.push({ name: taxiway.name, kind: "taxiway", offMap: true });
  network.nodes.push({ id, x: end.x, y: end.y, names: [taxiway.name, taxiway.runway] });
  network.edges.push({ from: end.id, to: id, name: taxiway.name, kind: "taxiway", pts: [[end.x, end.y], [end.x, end.y]] });
  const bundled = network.nodes.find((n) => n.id === end.id);
  bundled.names = [...new Set([...bundled.names, taxiway.name])];
}
fs.writeFileSync(outPath, JSON.stringify(network));
console.log(`wrote ${outPath}: ${network.lines.length} lines, ${network.nodes.length} nodes, ${network.edges.length} edges, ${network.releasePoints.length} release points`);
