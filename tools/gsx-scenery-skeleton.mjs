/**
 * Generates a gsx_sceneries.json skeleton from a GSX .ini profile.
 *
 * It fills in only what the profile actually states: the gate, the GSX stand
 * name, and the pushback labels that scenery offers. The release-point mapping
 * is left empty on purpose - which physical route a controller means by
 * "R/W" is local knowledge the profile does not contain.
 *
 *   node tools/gsx-scenery-skeleton.mjs <profile.ini> <scenery-name> [icao]
 */
import { readFileSync } from 'node:fs';
import { basename } from 'node:path';

const [file, scenery, icaoArg] = process.argv.slice(2);
if (!file || !scenery) {
  console.error('usage: node tools/gsx-scenery-skeleton.mjs <profile.ini> <scenery-name> [icao]');
  process.exit(1);
}

const icao = (icaoArg ?? basename(file).split(/[-.]/)[0]).toUpperCase();
const text = readFileSync(file, 'latin1');

/** "[gate a 31]" -> { stand: "Gate A31", controller: "A31" } */
function names(section) {
  const words = section.trim().split(/\s+/);
  const number = words.at(-1);
  const title = words.map((w) => w[0].toUpperCase() + w.slice(1));

  // GSX writes the BGL name as type + name + number, and a single-letter gate
  // name joins the number: "Gate A31", not "Gate A 31" (manual: "Gate A1",
  // "Parking2"). Multi-word names keep their spacing: "E Parking 112".
  const last = title.at(-2);
  const stand = last && last.length === 1
    ? [...title.slice(0, -2), last + number].join(' ')
    : title.join(' ');

  // Controllers write the ident without the parking-type prefix: "A31", "112".
  const prefix = words.slice(0, -1).filter((w) => w.length === 1).join('');
  return { stand, controller: (prefix + number).toUpperCase() };
}

const gates = {};
let current = null;

for (const raw of text.split(/\r?\n/)) {
  const line = raw.trim();
  const section = line.match(/^\[(.+)\]$/);
  if (section) {
    current = section[1] === 'general' ? null : names(section[1]);
    if (current) gates[current.controller] = { stand: current.stand, labels: [] };
    continue;
  }
  if (!current) continue;

  const labels = line.match(/^pushbacklabels\s*=\s*(.*)$/i);
  if (labels) {
    gates[current.controller].labels.push(
      ...labels[1].split('|').map((s) => s.trim()).filter(Boolean),
    );
  }
  // Extra slots carry their own label inside pushbackaddpos.
  const extra = line.match(/^pushbackaddpos\s*=\s*(.*)$/i);
  if (extra) {
    for (const m of extra[1].matchAll(/'label'\s*:\s*'([^']+)'/g)) {
      gates[current.controller].labels.push(m[1].trim());
    }
  }
}

/**
 * The config is keyed by the stand as controllers know it, which is not always
 * what the scenery calls it - Simnord's "[parking 89]" is FlightStrips' "F89".
 * Reconcile against the SAT stand list where one is given, and flag the rest
 * rather than guessing.
 */
const standsFile = process.argv[5];
const known = [];
if (standsFile) {
  for (const line of readFileSync(standsFile, 'latin1').split(/\r?\n/)) {
    const m = line.trim().match(/^STAND:[A-Z]{4}:([^:]+):/);
    if (m) known.push(m[1].trim().toUpperCase());
  }
}

function reconcile(ident) {
  if (!known.length || known.includes(ident)) return { key: ident };
  const matches = known.filter((s) => s.endsWith(ident));
  if (matches.length === 1) return { key: matches[0] };
  if (matches.length > 1) return { key: ident, review: `ambiguous: ${matches.join(' / ')}` };
  return { key: ident, review: 'no matching FlightStrips stand' };
}

const out = { icao, gates: {} };
const flagged = [];
for (const [ident, { stand, labels }] of Object.entries(gates).sort()) {
  const { key, review } = reconcile(ident);
  const entry = {
    stand,
    // Fill these in: "<release point>": "<one of the labels below>".
    points: {},
    available: [...new Set(labels)],
  };
  if (review) {
    entry.review = review;
    flagged.push(`${key}: ${review}`);
  }
  out.gates[key] = { [scenery]: entry };
}

console.log(JSON.stringify(out, null, 2));
if (flagged.length) {
  console.error(`\n${flagged.length} gate(s) need a human decision:`);
  for (const f of flagged) console.error(`  ${f}`);
}
