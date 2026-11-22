#!/usr/bin/env node
// Check a GSX airport profile against FlightStrips' release points.
//
//     node gsx-client/tools/audit.js path/to/EKCH-MyScenery.ini
//     node gsx-client/tools/audit.js path/to/EKCH-MyScenery.ini --all
//
// FlightStrips matches a pushback route by where it ends, not by its name. This
// reads every stand's routes from the .ini and reports, for each route, the
// release point it ends at. It flags:
//
//   MISLABEL   the name says one point, but the route ends at another
//   NO POINT   the name says a point, but the route ends at no known point
//   NO POS     the route has no position in the .ini (GSX computes it in the
//              sim), so it cannot be checked here
//
// --all also lists the routes that are fine. Exit code 1 when anything is
// flagged, so it can run in CI.

const fs = require("fs");
const path = require("path");

const args = process.argv.slice(2);
const iniPath = args.find((a) => !a.startsWith("--"));
if (!iniPath) {
  console.error("usage: audit.js <profile.ini> [--all] [--config gsx_sceneries.json]");
  process.exit(2);
}
const showAll = args.includes("--all");
const configArg = args.indexOf("--config");
const configPath = configArg >= 0
  ? args[configArg + 1]
  : path.join(__dirname, "..", "..", "backend", "config", "ekch", "gsx_sceneries.json");

const config = JSON.parse(fs.readFileSync(configPath, "utf8"));
const points = config.release_points || {};
const radius = config.release_point_radius_m || 40;

const metres = (a, b) => {
  const dLat = (a.lat - b.lat) * 111320;
  const dLon = (a.lon - b.lon) * 111320 * Math.cos((a.lat * Math.PI) / 180);
  return Math.hypot(dLat, dLon);
};
const nums = (s) => (s.match(/[+-]?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?/gi) || []).map(Number);
const words = (s) => String(s).toUpperCase().split(/[^A-Z0-9]+/).filter(Boolean);

// The point a route's name claims, if any: any word that is a known point.
const named = (label) => words(label).find((w) => Object.prototype.hasOwnProperty.call(points, w)) || null;

function extraRoutes(value) {
  // GSX stores a list of Python dictionaries. Keep strings whole so their
  // punctuation cannot be mistaken for dictionary or tuple delimiters.
  const tokens = value.match(/"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|[()[\]{},:]|[^()[\]{},:'"\s]+/g) || [];
  const routes = [];
  let depth = 0;
  let slotDepth = 0;
  let slot = null;

  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    if (token === "{" && slot === null) {
      slot = { label: "", pos: null };
      slotDepth = depth + 1;
    }
    if (slot !== null && depth === slotDepth) {
      if (token === "}") {
        routes.push(slot);
        slot = null;
      } else if (tokens[i + 1] === ":") {
        const key = token.slice(1, -1);
        const start = tokens[i + 2];
        if (key === "label" && /^['"]/.test(start)) {
          slot.label = start.slice(1, -1).replace(/\\(['"\\])/g, "$1");
        } else if (key === "pos" && (start === "(" || start === "[")) {
          const end = tokens.indexOf(start === "(" ? ")" : "]", i + 3);
          if (end >= 0) {
            const coords = tokens.slice(i + 3, end).filter((t) => t !== ",").map(Number);
            if (coords.length >= 2 && coords.every(Number.isFinite)) slot.pos = coords;
          }
        }
      }
    }
    if ("([{".includes(token)) depth++;
    if (")]}".includes(token)) depth--;
  }
  return routes;
}

function stands(file) {
  const out = [];
  let cur = null;
  for (const raw of fs.readFileSync(file, "latin1").split(/\r?\n/)) {
    const line = raw.trim();
    const sec = line.match(/^\[(.+)\]$/);
    if (sec) {
      cur = { name: sec[1].trim(), labels: [], mask: 3, left: null, right: null, extras: [] };
      out.push(cur);
      continue;
    }
    if (!cur) continue;
    const kv = line.match(/^([a-z_0-9]+)\s*=\s*(.*)$/i);
    if (!kv) continue;
    const key = kv[1].toLowerCase();
    const val = kv[2];
    if (key === "pushbacklabels") cur.labels = val.split("|").map((s) => s.trim());
    else if (key === "pushback") cur.mask = Number(val);
    else if (key === "pushbackleftpos") cur.left = nums(val);
    else if (key === "pushbackrightpos") cur.right = nums(val);
    else if (key === "pushbackaddpos") cur.extras.push(...extraRoutes(val));
  }
  return out
    .map((s) => {
      const routes = [];
      s.labels.forEach((label, i) => {
        if (i < 2 && s.mask & (1 << i)) routes.push({ label, pos: i === 0 ? s.left : s.right });
      });
      routes.push(...s.extras);
      return { name: s.name, routes };
    })
    .filter((s) => s.routes.length);
}

let flagged = 0;
let checked = 0;
for (const stand of stands(iniPath)) {
  const lines = [];
  for (const route of stand.routes) {
    checked++;
    const claim = named(route.label);
    if (!route.pos) {
      if (claim) lines.push(`  NO POS     ${route.label}`);
      continue;
    }
    const end = { lat: route.pos[0], lon: route.pos[1] };
    let at = null;
    let best = Infinity;
    for (const [name, spot] of Object.entries(points)) {
      const d = metres(end, spot);
      if (d < best) [best, at] = [d, name];
    }
    const reaches = best <= radius ? at : null;
    if (claim && reaches && claim !== reaches) {
      lines.push(`  MISLABEL   ${route.label}  ends at ${reaches} (${Math.round(best)} m), not ${claim}`);
    } else if (claim && !reaches) {
      lines.push(`  NO POINT   ${route.label}  nearest is ${at}, ${Math.round(best)} m away`);
    } else if (showAll) {
      lines.push(`  ok         ${route.label}${reaches ? `  -> ${reaches}` : "  (no point: reachable by direction only)"}`);
    }
  }
  const problems = lines.filter((l) => !l.startsWith("  ok")).length;
  flagged += problems;
  if (lines.length) console.log(`[${stand.name}]\n${lines.join("\n")}`);
}

console.log(`\n${checked} routes checked against ${Object.keys(points).length} release points (radius ${radius} m): ${flagged} flagged`);
process.exit(flagged ? 1 : 0);
