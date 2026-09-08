/**
 * Infers gsx_sceneries.json "points" by matching GSX pushback labels against
 * FlightStrips RELEASE_POINTS. A GSX label is "<taxiway> Face <dir>"; the
 * taxiway part is the release point a controller picks in PushbackMapDialog.
 */
import { readFileSync, writeFileSync } from 'node:fs';

const [iniPath, tsPath] = process.argv.slice(2);

// FlightStrips release points, from the frontend config.
const ts = readFileSync(tsPath, 'utf8');
const block = ts.slice(ts.indexOf('export const RELEASE_POINTS'));
const release = new Set(
  [...block.slice(0, block.indexOf('];')).matchAll(/label:\s*"([^"]+)"/g)].map((m) => m[1].toUpperCase()),
);

// GSX labels per stand.
const norm = (s) => s.trim().replace(/\s+/g, ' ');
const stands = {};
let cur = null;
for (const raw of readFileSync(iniPath, 'latin1').split(/\r?\n/)) {
  const line = raw.trim();
  const sec = line.match(/^\[(.+)\]$/);
  if (sec) { cur = sec[1] === 'general' ? null : sec[1]; if (cur) stands[cur] = []; continue; }
  if (!cur) continue;
  const lab = line.match(/^pushbacklabels\s*=\s*(.*)$/i);
  if (lab) stands[cur].push(...lab[1].split('|').map(norm).filter(Boolean));
  const add = line.match(/^pushbackaddpos\s*=\s*(.*)$/i);
  if (add) for (const m of add[1].matchAll(/'label'\s*:\s*'([^']+)'/g)) stands[cur].push(norm(m[1]));
}

/**
 * "Z2 Face E" -> "Z2". Also tolerates the variations the profile actually
 * contains: a TWY prefix, a trailing qualifier in parentheses ("(A380)",
 * "(<CAT-C)", "(Push & Hold)"), and a stray full stop.
 */
function toReleasePoint(label) {
  const cleaned = label
    .replace(/^TWY\s+/i, '')
    .replace(/\s*\([^)]*\)\s*$/, '')
    .replace(/\.\s*$/, '')
    .trim();
  const m = cleaned.match(/^([A-Z]+[0-9]*)(?:\s+FACE\s+[NSEW]{1,2})?$/i);
  if (!m) return null;
  const token = m[1].toUpperCase();
  return release.has(token) ? token : { unknown: token };
}

let matched = 0, generic = 0, unknownToken = 0, unparsed = 0;
const unknowns = new Map(), shapes = new Map();
for (const labels of Object.values(stands)) {
  for (const label of labels) {
    if (/Nose (Right|Left)\/Tail/i.test(label)) { generic++; continue; }
    const r = toReleasePoint(label);
    if (r === null) { unparsed++; shapes.set(label, (shapes.get(label) ?? 0) + 1); }
    else if (typeof r === 'object') { unknownToken++; unknowns.set(r.unknown, (unknowns.get(r.unknown) ?? 0) + 1); }
    else matched++;
  }
}

const total = matched + generic + unknownToken + unparsed;
console.log(`FlightStrips RELEASE_POINTS: ${release.size}`);
console.log(`GSX labels across ${Object.keys(stands).length} stands: ${total}\n`);
console.log(`  matched a release point : ${matched}`);
console.log(`  generic Nose/Tail       : ${generic}  (no named route - correctly unmappable)`);
console.log(`  taxiway not in FS list  : ${unknownToken}`);
console.log(`  label shape unrecognised: ${unparsed}`);
if (unknowns.size) console.log(`\n  unknown taxiways: ${[...unknowns].map(([k, v]) => `${k}(${v})`).join(' ')}`);
if (shapes.size) console.log(`\n  unrecognised shapes: ${[...shapes].slice(0, 12).map(([k, v]) => `"${k}"(${v})`).join(' ')}`);

// ---------------------------------------------------------------------------
// Merge into an existing gsx_sceneries.json when one is given as argv[4].
// ---------------------------------------------------------------------------

const configPath = process.argv[4];
const sceneryName = process.argv[5];
if (!configPath || !sceneryName) process.exit(0);

const config = JSON.parse(readFileSync(configPath, 'utf8'));

/** "[gate a 31]" -> "Gate A31", matching how the skeleton names stands. */
function standName(section) {
  const words = section.trim().split(/\s+/);
  const title = words.map((w) => w[0].toUpperCase() + w.slice(1));
  const last = title.at(-2);
  return last && last.length === 1
    ? [...title.slice(0, -2), last + words.at(-1)].join(' ')
    : title.join(' ');
}

// Index the config's gates by the stand name the scenery uses, since the
// config keys are FlightStrips idents and the .ini sections are not.
const byStand = new Map();
for (const [gate, sceneries] of Object.entries(config.gates)) {
  const entry = sceneries[sceneryName];
  if (entry?.stand) byStand.set(entry.stand, entry);
}

let filled = 0, multi = 0, unmapped = 0, missing = 0;
const unmappedByStand = [];

for (const [section, labels] of Object.entries(stands)) {
  const entry = byStand.get(standName(section));
  if (!entry) { missing++; continue; }

  // Every route reaching a release point, in the order the profile lists them
  // (the two defaults first, then extra slots). A stand offering the same
  // taxiway in two facings yields both: narrowing the pilot's menu to the pair
  // still guarantees they leave via the taxiway the controller named, and the
  // facing stays their choice.
  const points = {};
  const leftovers = [];

  for (const label of labels) {
    if (/Nose (Right|Left)\/Tail/i.test(label)) continue;
    const r = toReleasePoint(label);
    if (typeof r !== 'string') { leftovers.push(label); continue; }
    points[r] = points[r] ?? [];
    if (!points[r].includes(label)) points[r].push(label);
  }

  for (const routes of Object.values(points)) if (routes.length > 1) multi++;

  entry.points = points;
  filled += Object.keys(points).length;

  const notes = [];
  if (leftovers.length) {
    unmapped += leftovers.length;
    notes.push(`unmapped routes: ${leftovers.join(' | ')}`);
    unmappedByStand.push(`${standName(section)}: ${leftovers.join(' | ')}`);
  }

  // Keep any note the skeleton left about reconciling the stand ident.
  const existing = (entry.review ?? '')
    .split(' ; ')
    .filter((n) => n && !n.startsWith('unmapped routes') && !n.startsWith('ambiguous release points'));
  const merged = [...existing, ...notes];
  if (merged.length) entry.review = merged.join(' ; ');
  else delete entry.review;
}

writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`);

console.log(`\n--- merged into ${configPath} ---`);
console.log(`  points written        : ${filled}`);
console.log(`  stands not in config  : ${missing}`);
console.log(`  points with 2+ routes : ${multi}  (both facings kept - pilot picks)`);
console.log(`  routes left unmapped  : ${unmapped}`);
if (unmappedByStand.length) {
  console.log(`\n  needs a human (${unmappedByStand.length} stands):`);
  for (const u of unmappedByStand.slice(0, 8)) console.log(`    ${u}`);
  if (unmappedByStand.length > 8) console.log(`    ... and ${unmappedByStand.length - 8} more`);
}
