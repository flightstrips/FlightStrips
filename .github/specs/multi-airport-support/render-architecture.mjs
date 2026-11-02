/**
 * Render the Markdown source as a standalone, offline HTML reading edition.
 * Requires Node >=20 and marked 17. Install marked in your Node environment, or:
 * node render-architecture.mjs --marked-module /absolute/path/to/marked.esm.js
 * Output is deterministic; no network or browser is needed to regenerate it.
 */
import { readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { createRequire } from 'node:module';
import { fileURLToPath, pathToFileURL } from 'node:url';
import path from 'node:path';

const directory = path.dirname(fileURLToPath(import.meta.url));
const moduleArgument = process.argv.indexOf('--marked-module');
if (moduleArgument >= 0 && !process.argv[moduleArgument + 1]) {
  throw new Error('--marked-module requires a module path');
}
const require = createRequire(import.meta.url);
const modulePath = moduleArgument >= 0
  ? path.resolve(process.argv[moduleArgument + 1])
  : require.resolve('marked');
const { Marked } = await import(pathToFileURL(modulePath).href);
const source = (await readFile(path.join(directory, 'architecture-proposal.md'), 'utf8')).replace(/\r\n/g, '\n');
const sourceHash = createHash('sha256').update(source).digest('hex');
const schemaContract = (await readFile(path.join(directory, 'schema-contract.md'), 'utf8')).replace(/\r\n/g, '\n');
const radisReview = (await readFile(path.join(directory, 'radis-prototype-review.md'), 'utf8')).replace(/\r\n/g, '\n');
const handlerDesign = (await readFile(path.join(directory, 'handler-design.md'), 'utf8')).replace(/\r\n/g, '\n');
const planningReview = (await readFile(path.join(directory, 'planning-readiness.md'), 'utf8')).replace(/\r\n/g, '\n');
const epicPlan = (await readFile(path.join(directory, 'epic-plan.md'), 'utf8')).replace(/\r\n/g, '\n');
const ekchExample = await readFile(path.join(directory, 'examples/ekch.airport.yaml'), 'utf8');
const fakeExample = await readFile(path.join(directory, 'examples/zzzz.airport.yaml'), 'utf8');
const proofPatch = (await readFile(path.join(directory, 'examples/handler-proof.patch'), 'utf8')).replace(/\r\n/g, '\n');
const handlerPatch = proofPatch.split('diff --git a/backend/internal/airportworkflow/handlers.go b/backend/internal/airportworkflow/handlers.go\n')[1]?.split('diff --git ')[0];
if (!handlerPatch) throw new Error('Handler source missing from proof artifact');
const handlerCode = handlerPatch.split('\n').filter(line => line.startsWith('+') && !line.startsWith('+++')).map(line => line.slice(1)).join('\n');
const configurationSchema = await readFile(path.join(directory, 'schema/airport-configuration.schema.json'), 'utf8');
const escape = value => String(value).replace(/[&<>"']/g, char => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
}[char]));
const slug = value => value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

const palette = {
  blue: ['#eef3ff', '#bdcef6', '#193e82'],
  teal: ['#e8f6f1', '#a8d9cb', '#165b4b'],
  slate: ['#f3f5f8', '#d5dce5', '#304156'],
  amber: ['#fff5e7', '#e7cba0', '#845016'],
};
function box(x, y, width, height, heading, lines = [], tone = 'blue') {
  const [fill, stroke, ink] = palette[tone];
  return `<g><rect x="${x}" y="${y}" width="${width}" height="${height}" rx="12" fill="${fill}" stroke="${stroke}"/>
    <text x="${x + 18}" y="${y + 29}" fill="${ink}" class="node-title">${escape(heading)}</text>
    ${lines.map((line, i) => `<text x="${x + 18}" y="${y + 54 + i * 22}" class="node-detail">${escape(line)}</text>`).join('')}</g>`;
}
function arrow(name, d, { dashed = false, tone = '#73839c', both = false } = {}) {
  return `<path d="${d}" fill="none" stroke="${tone}" stroke-width="2" ${dashed ? 'stroke-dasharray="6 5"' : ''} marker-end="url(#arrow-${name})" ${both ? `marker-start="url(#back-${name})"` : ''}/>`;
}
function diagram(name, title, description, width, height, contents) {
  return `<svg class="diagram" viewBox="0 0 ${width} ${height}" role="img" aria-labelledby="title-${name} desc-${name}" xmlns="http://www.w3.org/2000/svg">
  <title id="title-${name}">${escape(title)}</title><desc id="desc-${name}">${escape(description)}</desc>
  <defs><marker id="arrow-${name}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#73839c"/></marker>
  <marker id="back-${name}" viewBox="0 0 10 10" refX="1" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M 10 0 L 0 5 L 10 10 z" fill="#73839c"/></marker></defs>${contents}</svg>`;
}
function figure(name, title, description, width, height, content, caption) {
  return `<figure class="architecture-figure" id="figure-${name}"><div class="figure-label">${escape(title)}</div><div class="diagram-scroll" tabindex="0" aria-label="${escape(title)}; scroll horizontally on small screens">${diagram(name, title, description, width, height, content)}</div><figcaption>${escape(caption)}</figcaption></figure>`;
}

const figures = {};
{
  const n = 'components';
  figures[n] = figure(n, '01 / Architecture at a glance',
    'Shared feeds, an immutable airport registry, and application services including shared PDC sit outside session runtimes. One transport connects memberships. The session executor calls its bound pure handler for decisions, invokes shared services with context, and routes human commands to the initiating plugin.', 1000, 580,
    `${arrow(n, 'M200 240 H240')}${arrow(n, 'M425 240 H465')}${arrow(n, 'M715 240 H750')}
    ${arrow(n, 'M580 110 V185')}${arrow(n, 'M650 65 H680 V150 H805 V185')}
    ${arrow(n, 'M300 65 H325 V150 H520 V185')}${arrow(n, 'M160 20 V5 H990 V225 H985')}
    ${arrow(n, 'M865 110 V185', { both: true })}
    ${arrow(n, 'M865 310 V365', { both: true })}${arrow(n, 'M690 420 H730 V280 H750', { both: true })}
    ${arrow(n, 'M980 300 H995 V530 H335 V475')}${arrow(n, 'M335 365 V300')}
    ${box(20, 190, 180, 110, 'EuroScope', ['One running instance', 'Observe callbacks once'])}
    ${box(245, 190, 180, 110, 'Transport', ['One authenticated', 'protobuf connection'])}
    ${box(470, 190, 245, 110, 'Membership coordinator', ['Coverage + tuned radios', 'Attach / detach per airport'])}
    ${box(755, 190, 225, 120, 'Session core / executor', ['Independent master + sync', 'Strip state + validated effects'], 'teal')}
    ${box(355, 20, 295, 90, 'Immutable airport registry', ['Combined config + bound handler', 'Versioned definition'], 'slate')}
    ${box(20, 20, 280, 90, 'Shared VATSIM sources', ['One cache per upstream feed', 'Read-only observations'], 'slate')}
    ${box(700, 20, 280, 90, 'Shared application services', ['PDC: one client / logon / worker', 'Operational state partitioned'], 'slate')}
    ${box(755, 370, 225, 105, 'Bound coded handler', ['Stateless decision logic', 'No database / transport I/O'], 'teal')}
    ${box(470, 370, 220, 105, 'Frontend tab', ['One selected session', 'Coded airport view'], 'slate')}
    ${box(245, 370, 180, 105, 'Controller router', ["Initiator's own plugin", 'No master fallback'])}`,
    'Global resources stay application-owned. Session cores own state and execute decisions; handlers do no I/O. Automatic work may use the same session master or assuming controller (D4). These are modules in the existing backend, not additional deployments.');
}
{
  const n = 'memberships';
  figures[n] = figure(n, '02 / One connection, independent airport sessions',
    'A EuroScope instance has one persistent transport and two independent airport memberships. Example membership roles differ; actual airport priorities remain open.', 1000, 385,
    `${arrow(n, 'M500 105 V130 H255 V150')}${arrow(n, 'M500 130 H745 V150')}
    ${box(340, 20, 320, 85, 'One EuroScope connection', ['Authenticated instance + environment'])}
    ${box(35, 155, 445, 195, 'EKBI / airport session', ['Membership role: master (example)', 'Independent strips, bays, and order', 'Own sync state and master epoch', 'Own service-state partitions'], 'teal')}
    ${box(520, 155, 445, 195, 'EKYT / airport session', ['Membership role: non-master (example)', 'Independent strips, bays, and order', 'Own sync state and master epoch', 'Own service-state partitions'])}`,
    'Roles are illustrative, not operating priorities. The same flight may have a separate strip in each airport session; bay movements are not mirrored.');
}
{
  const n = 'coverage';
  figures[n] = figure(n, '03 / Coverage changes one membership at a time',
    'EKDK serves EKBI. A local tower alone leaves EKDK providing approach. When approach takes over EKDK leaves at backend attachment or the grace deadline. When approach leaves EKDK automatically rejoins. The control transport remains connected throughout.', 1000, 390,
    `${arrow(n, 'M240 150 H260')}${arrow(n, 'M490 150 H510')}${arrow(n, 'M740 150 H760')}
    ${box(15, 70, 225, 165, 'A. EKDK serves EKBI', ['Membership active', 'Responsibilities from', 'the airport coverage policy'], 'teal')}
    ${box(265, 70, 225, 165, 'B. Only tower joins', ['EKDK still provides APP', 'Keep EKBI membership', 'Reevaluate tuned radios'], 'teal')}
    ${box(515, 70, 225, 165, 'C. APP takes over', ['EKDK leaves EKBI', 'On backend attachment', 'or grace deadline'], 'amber')}
    ${box(765, 70, 220, 165, 'D. APP leaves', ['Coverage returns to EKDK', 'Reattach automatically', 'Sync before editing'], 'teal')}
    <path d="M125 240 V285 M375 240 V285 M625 240 V285 M875 240 V285" fill="none" stroke="#bdc8d9" stroke-dasharray="5 5"/>
    ${box(15, 290, 970, 80, 'The physical control connection remains available', ['Removing EKBI does not reset another airport. A readiness gap uses the existing read-only board.'], 'slate')}`,
    'Backend attachment and full synchronization are separate. No grace duration or airport-specific responsibility list is selected here.');
}
{
  const n = 'commands';
  const xs = [100, 365, 645, 900];
  const messages = [
    [100, 365, 130, 'Action + bound session + strip version'],
    [365, 645, 205, 'Validate; route to bound own membership'],
    [645, 900, 280, 'Apply EuroScope operation'],
    [900, 645, 345, 'Result / observation'],
    [645, 365, 405, 'Scoped, correlated result / observation'],
    [365, 100, 465, 'Reconcile outcome and publish update'],
  ];
  figures[n] = figure(n, '04 / Commands use the initiating controller’s plugin',
    'A frontend action is validated within its session, dispatched to its own plugin, applied in EuroScope, and reconciled from correlated results or observations. A disconnected own plugin blocks the action even if another master exists.', 1000, 580,
    `${xs.map(x => `<path d="M${x} 80 V490" stroke="#c4cedc" stroke-dasharray="5 6"/>`).join('')}
    ${box(15, 15, 170, 60, 'Frontend tab', [], 'slate')}
    ${box(250, 15, 230, 60, 'Session + workflow')}
    ${box(540, 15, 210, 60, 'Own plugin', [], 'teal')}
    ${box(815, 15, 170, 60, 'EuroScope', [], 'slate')}
    ${messages.map(([from, to, y, label]) => `${arrow(n, `M${from} ${y} H${to}`)}<text x="${Math.min(from, to) + 10}" y="${y - 12}" class="sequence-label">${escape(label)}</text>`).join('')}
    ${box(45, 510, 910, 60, 'Own plugin disconnected → block edits; never reroute to the airport master', [], 'amber')}`,
    'Human commands retain own-plugin routing. Reuse existing EKCH move/error/reconciliation behavior (D3), without a new acknowledgement gate or pending framework. Automatic work may use the session master or assuming controller (D4).');
}
{
  const n = 'configuration';
  figures[n] = figure(n, '05 / Compile reusable definitions; isolate live state',
    'Compiled handlers and one combined airport file feed startup validation and binding. The resulting immutable airport definition supplies independent session runtimes. Frontend views remain code.', 1000, 490,
    `${arrow(n, 'M290 105 H330 V220 H375')}${arrow(n, 'M290 235 H375')}${arrow(n, 'M290 365 H330 V270 H375')}
    ${arrow(n, 'M630 245 H715')}${arrow(n, 'M850 310 V375')}${arrow(n, 'M630 425 H715', { both: true })}
    ${box(15, 60, 275, 90, 'Compiled handler registry', ['EKCH, shared RADIS, or custom', 'Typed actions and decisions'])}
    ${box(15, 190, 275, 90, 'One airport YAML file', ['Coverage, master, handler + bays', 'Typed settings, services, views'])}
    ${box(15, 320, 275, 90, 'Referenced domain datasets', ['Stands, AMAN, CDM tables', 'Keep existing domain formats'])}
    ${box(380, 175, 250, 150, 'Validate + bind at startup', ['Known IDs and typed settings', 'References and dependencies', 'One effective revision'], 'slate')}
    ${box(720, 190, 265, 120, 'Immutable airport definition', ['Inject explicit airport context', 'No current-airport global'], 'teal')}
    ${box(720, 380, 265, 90, 'Airport session runtimes', ['Independent board + services', 'Isolated LIVE / training state'], 'teal')}
    ${box(380, 380, 250, 90, 'Frontend views stay in code', ['Share RADIS where appropriate', 'New airports may need UI code'], 'slate')}`,
    'One combined file per airport selects compiled code. Session events use the already bound handler; configuration parsing and factory resolution happen at startup.');
}
{
  const n = 'handlers';
  figures[n] = figure(n, '08 / Same event, different bound handlers',
    'EKCH and a fictional paper workflow receive a TAXI observation through separate session cores. Their stateless bound handlers choose TAXI and HOLD respectively. Core validation keeps revisions, state and command routing independent. Two sessions may share a paper handler while keeping separate state.', 1000, 390,
    `${arrow(n, 'M225 110 H275')}${arrow(n, 'M545 110 H590')}${arrow(n, 'M225 265 H275')}${arrow(n, 'M545 265 H590')}
    ${box(15, 65, 210, 100, 'Same TAXI observation', ['Session: EKCH / LIVE', 'Callsign: DEMO123'], 'slate')}
    ${box(280, 65, 265, 100, 'EKCH handler', ['Existing Go bay functions', 'Decision: TAXI bay'])}
    ${box(595, 65, 390, 100, 'EKCH session core', ['Own revision + independent strip state', 'Own-plugin intents after core guards'], 'teal')}
    ${box(15, 220, 210, 100, 'Same TAXI observation', ['Session: ZZZZ / LIVE', 'Callsign: DEMO123'], 'slate')}
    ${box(280, 220, 265, 100, 'Fictional paper handler', ['Typed settings + custom code', 'Decision: HOLD bay'])}
    ${box(595, 220, 390, 100, 'ZZZZ session core', ['Own revision + independent strip state', 'Shared handler code; no shared board'], 'teal')}
    <text x="20" y="365" class="node-detail">Immutable handler instances may be shared. Mutable strips, annotations, revisions, and locks belong to sessions.</text>`,
    'The executable proof loads both complete configs together. ZZZZ is fictional example data; production application wiring and complete EKCH migration remain future work.');
}
{
  const n = 'clearance';
  figures[n] = figure(n, '06 / Why a bay is not a clearance state',
    'In the ENGM draft, dragging a departure from taxi into the runway line means lineup. A clearance-box or runway-bar action enters the same runway line with takeoff clearance. The trigger and clearance effect therefore remain separate from the destination bay.', 1000, 310,
    `${arrow(n, 'M305 75 L375 115')}${arrow(n, 'M305 235 L375 195')}
    ${arrow(n, 'M625 115 L700 75')}${arrow(n, 'M625 195 L700 235')}
    ${box(15, 25, 290, 100, 'Drag departure to runway', ['Semantic action: line up'])}
    ${box(15, 185, 290, 100, 'Clearance-box / runway bar', ['Semantic action: takeoff clearance'], 'teal')}
    ${box(380, 85, 245, 140, 'RUNWAY LINE', ['Same destination bay', 'Different action effects'], 'slate')}
    ${box(705, 25, 280, 100, 'Lineup indication', ['Clearance effect from action'])}
    ${box(705, 185, 280, 100, 'Takeoff clearance', ['Clearance effect from action'], 'teal')}`,
    'ENGM draft behavior is a design input, not approved operating configuration. A single bay-to-EuroScope-state lookup cannot express this distinction.');
}

{
  const n = 'vatsim';
  figures[n] = figure(n, '07 / Shared feeds, independent airport projections',
    'The VATSIM pilot/prefile and transceiver endpoints each feed one application-owned cache pipeline. Shared consumers and the EKBI and EKYT projections read the same immutable source generations. Each airport keeps its own operational results; adding a session does not create another fetcher.', 1000, 505,
    `${arrow(n, 'M275 95 H310 V155 H345')}${arrow(n, 'M275 225 H345')}
    ${arrow(n, 'M630 145 H675 V65 H710')}${arrow(n, 'M630 185 H675 V235 H710')}
    ${arrow(n, 'M630 235 H655 V405 H710')}
    ${box(15, 45, 260, 95, 'VATSIM pilot / prefile feed', ['One fetch / parse pipeline', 'Shared by all LIVE consumers'])}
    ${box(15, 175, 260, 95, 'VATSIM transceiver feed', ['One fetch / parse pipeline', 'Shared tuned-radio evidence'])}
    ${box(350, 110, 280, 170, 'Shared application source', ['Read-only source generations', 'Common timestamp + health', 'Shared indexes and refresh policy', 'Snapshot reads do not fetch'], 'slate')}
    ${box(715, 20, 270, 95, 'Other shared consumers', ['Coverage, identity, EFB, etc.', 'Read the same source'])}
    ${box(715, 175, 270, 125, 'EKBI session projection', ['Relevant flights + airport rules', 'Own strips, bays, stand state', 'No full feed copy or poller'], 'teal')}
    ${box(715, 345, 270, 125, 'EKYT session projection', ['Relevant flights + airport rules', 'Own strips, bays, stand state', 'No full feed copy or poller'], 'teal')}
    ${box(15, 345, 600, 125, 'Source lifetime follows aggregate application demand', ['Adding or removing a session does not recreate or evict the source.', 'A waiting LIVE control instance can still need staffing / radio data.', 'Training and playback remain isolated from the public LIVE feed.'], 'slate')}`,
    'This preserves today’s shared source assembly. Fetching and caching are application-scoped; only the relevant projections and operational results belong to airport sessions.');
}

{
  const n = 'pdc';
  figures[n] = figure(n, '09 / Shared PDC service, separate airport clearances',
    'One application PDC service uses one provider client, HTTP transport, logon key and polling coordinator. Separate airport callsigns address distinct mailboxes. Requests and acknowledgements are routed to the owning airport session validation and clearance state.', 1000, 380,
    `${arrow(n, 'M255 185 H280 V105 H310')}${arrow(n, 'M280 185 V270 H310')}
    ${arrow(n, 'M575 105 H625')}${arrow(n, 'M575 270 H625')}
    ${box(15, 125, 240, 130, 'Shared application PDC', ['One provider client / transport', 'One deployment HOPPIE_LOGON', 'One polling coordinator'], 'slate')}
    ${box(315, 55, 260, 105, 'EKCH mailbox', ['Airport sender / poll callsign', 'Same shared provider key'])}
    ${box(315, 220, 260, 105, 'Another enabled airport', ['Its own sender / poll callsign', 'Same shared provider key'])}
    ${box(630, 55, 350, 105, 'EKCH session context', ['Own validation and permissions', 'Clearances, replies, revisions, timeouts'], 'teal')}
    ${box(630, 220, 350, 105, 'Other airport session context', ['Own validation and permissions', 'Clearances, replies, revisions, timeouts'], 'teal')}`,
    'Shared credentials do not merge airport identities or clearance state. The coordinator polls distinct live mailboxes as needed, without creating a client or polling loop per session/tab.');
}

{
  const n = 'epics';
  figures[n] = figure(n, '10 / Epic prerequisites and parallel work',
    'Airport registry precedes coded EKCH workflows. Plugin memberships and shared services then proceed in parallel. Memberships feed frontend binding; both transport and shared services feed coverage. Coverage, frontend binding and services converge on integrated RADIS, followed by acceptance and offline activation.', 1000, 590,
    `${arrow(n, 'M300 75 H355')}${arrow(n, 'M640 75 H695')}
    ${arrow(n, 'M500 125 V215')}${arrow(n, 'M840 125 V215')}
    ${arrow(n, 'M640 270 H695')}${arrow(n, 'M700 100 H670 V175 H160 V215')}
    ${arrow(n, 'M160 320 V370 H500 V415')}${arrow(n, 'M500 320 V415')}
    ${arrow(n, 'M840 320 V370 H500 V415')}${arrow(n, 'M640 470 H695')}
    ${box(20, 25, 280, 100, 'MA-01 / Airport registry', ['One combined config per airport', 'Full production loading'])}
    ${box(360, 25, 280, 100, 'MA-02 / Workflow handlers', ['Real EKCH paths + shared executor', 'Typed decisions; existing behavior'])}
    ${box(700, 25, 280, 100, 'MA-03 / Plugin memberships', ['One transport; independent sync', 'Scoped roles and command routing'], 'teal')}
    ${box(20, 220, 280, 100, 'MA-06 / Frontend sessions', ['Per-tab selection + own binding', 'Existing offline/read-only behavior'], 'slate')}
    ${box(360, 220, 280, 100, 'MA-05 / Shared services', ['One acquisition/client owner', 'Scoped calculations + state'], 'teal')}
    ${box(700, 220, 280, 100, 'MA-04 / Coverage and rejoin', ['Responsibilities + tuned radios', 'Takeover and automatic return'], 'teal')}
    ${box(360, 420, 280, 100, 'MA-07 / Integrated RADIS', ['Full agreed features + restoration', 'Authoritative boards and ES effects'])}
    ${box(700, 420, 280, 100, 'MA-08 / Validate and activate', ['Integrated + operational acceptance', 'Coordinated offline upgrade'], 'amber')}
    <text x="20" y="565" class="node-detail">Arrows are prerequisites for completion. Airport data collection and acceptance preparation start during the foundations.</text>`,
    'MA-03/05 can proceed together, then MA-04/06. RADIS depends on those foundations. Final activation also requires approved airport data; fictional fixtures do not become deployment definitions.');
}

const markdown = new Marked({ gfm: true });
markdown.use({ renderer: {
  heading(token) {
    return `<h${token.depth} id="${slug(token.text)}">${markdown.parseInline(token.text)}</h${token.depth}>`;
  },
  code(token) {
    if (token.lang === 'mermaid') {
      if (token.text.includes('MA01[MA-01 Airport registry]')) return figures.epics;
      if (token.text.includes('C[Shared VATSIM source]')) return figures.vatsim;
      if (!token.text.includes('Membership and coverage coordinator')) {
        throw new Error('Unknown Mermaid diagram: add an offline SVG rendering before publishing');
      }
      return figures.components;
    }
    return `<pre><code${token.lang ? ` class="language-${escape(token.lang)}"` : ''}>${escape(token.text)}</code></pre>`;
  },
} });
const tokens = markdown.lexer(source);
const sections = [];
let preamble = [];
for (const token of tokens) {
  if (token.type === 'heading' && token.depth === 1) continue;
  if (token.type === 'heading' && token.depth === 2) {
    sections.push({ title: token.text, id: slug(token.text), tokens: [] });
  } else if (sections.length) sections.at(-1).tokens.push(token);
  else preamble.push(token);
}
const inline = value => markdown.parseInline(value);
const insertions = {
  'state-and-identity-model': figures.memberships,
  'coverage-and-membership-lifecycle': figures.coverage,
  'commands-observations-and-concurrent-editing': figures.commands,
  'repository-configuration-and-workflow-model': figures.configuration,
};
const contents = sections.map((section, index) => {
  let body = markdown.parser(section.tokens);
  if (section.id === 'confirmed-scope-and-implementation-readiness') body = '<span id="open-values-and-follow-up-decisions" aria-hidden="true"></span>' + body;
  if (section.id === 'frontend-session-discovery-and-navigation') {
    body += `<div class="schema-appendices"><details class="provenance"><summary>Read the RADIS prototype review and integration implications</summary>${markdown.parse(radisReview.replace(/^# RADIS prototype: evidence and integration implications\s*/u, ''))}</details></div>`;
  }
  if (section.id === 'reusable-services-and-runtime-isolation') {
    const marker = '<h3 id="shared-pdc-transport-and-scoped-clearance-processing">Shared PDC transport and scoped clearance processing</h3>';
    if (!body.includes(marker)) throw new Error('PDC diagram insertion point changed');
    body = body.replace(marker, marker + figures.pdc);
  }
  if (insertions[section.id]) {
    const position = body.indexOf('</p>') + 4;
    if (position < 4) throw new Error(`Missing opening paragraph in ${section.id}`);
    body = body.slice(0, position) + insertions[section.id] + body.slice(position);
  }
  if (section.id === 'confirmed-scope-and-implementation-readiness') {
    body += `<div class="schema-appendices"><details class="provenance"><summary>Read the consistency audit and confirmed delivery decisions</summary>${markdown.parse(planningReview.replace(/^# Proposal consistency review and planning readiness\s*/u, ''))}</details></div>`;
  }
  if (section.id === 'implementation-epics') {
    body = body.replaceAll('href="epic-plan.md#', 'href="#');
    const firstEpic = epicPlan.indexOf('## MA-01:');
    if (firstEpic < 0) throw new Error('Epic detail insertion point changed');
    body += `<div class="schema-appendices"><details class="provenance" open><summary>Read all eight epic drafts, child issues, acceptance criteria, and release gates</summary>${markdown.parse(epicPlan.slice(firstEpic).replace(/^## /gm, '### '))}</details></div>`;
  }
  if (section.id === 'repository-configuration-and-workflow-model') {
    const marker = '<p>ENGM requires';
    if (!body.includes(marker)) throw new Error('ENGM diagram insertion point changed');
    body = body.replace(marker, figures.clearance + marker);
    const handlerMarker = '<h3 id="executable-handler-proof-and-its-limits">';
    if (!body.includes(handlerMarker)) throw new Error('Handler diagram insertion point changed');
    body = body.replace(handlerMarker, figures.handlers + handlerMarker);
    body += `<div class="schema-appendices"><details class="provenance"><summary>Read the handler design, proof, and reproduction commands</summary>${markdown.parse(handlerDesign.replace(/^# Coded workflow handlers: contract and executable proof\s*/u, ''))}</details><details class="provenance"><summary>Inspect the actual sample Go handlers</summary><pre class="large-source"><code>${escape(handlerCode)}</code></pre></details><details class="provenance"><summary>Inspect the complete fictional ZZZZ configuration</summary><p>Invented values for the executable architecture example. Never deploy this airport.</p><pre class="large-source"><code>${escape(fakeExample)}</code></pre></details><details class="provenance"><summary>Read the complete configuration schema contract</summary>${markdown.parse(schemaContract.replace(/^# Airport configuration schema v1\s*/u, ''))}</details><details class="provenance"><summary>Inspect the complete, validated EKCH airport example</summary><p>This worked migration preserves every current EKCH YAML setting and includes its handler and bays. The runtime continues to use the existing configuration.</p><pre class="large-source"><code>${escape(ekchExample)}</code></pre></details><details class="provenance"><summary>Inspect the machine-readable JSON Schema</summary><pre class="large-source"><code>${escape(configurationSchema)}</code></pre></details></div>`;
  }
  return `<section class="proposal-section" id="${section.id}" aria-labelledby="heading-${section.id}"><div class="section-heading"><span class="section-number">${String(index + 1).padStart(2, '0')}</span><h2 id="heading-${section.id}">${inline(section.title)}</h2><a class="section-link" href="#${section.id}" aria-label="Link to ${escape(section.title)}">#</a></div>${body}</section>`;
}).join('\n');
const toc = sections.map((s, i) => `<a href="#${s.id}"><span>${String(i + 1).padStart(2, '0')}</span>${escape(s.title)}</a>`).join('\n');

const html = `<!doctype html>
<!-- Generated by render-architecture.mjs. Edit architecture-proposal.md, then regenerate.
     Markdown SHA-256: ${sourceHash} -->
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light"><meta name="description" content="FlightStrips multi-airport architecture proposal: sessions, coverage, workflows, services, and a compatible migration path.">
<title>Multi-airport architecture · FlightStrips</title>
<style>
:root{--page:#f5f6fa;--paper:#fff;--ink:#1c2b40;--muted:#607086;--line:#e0e6ee;--blue:#2758a5;--teal:#15735d;--nav:255px;font-family:Inter,Segoe UI,Arial,sans-serif;color:var(--ink);background:var(--page);font-synthesis:none;line-height:1.7}
*{box-sizing:border-box}html{scroll-behavior:smooth;scroll-padding-top:32px}body{margin:0}a{color:var(--blue);text-decoration-thickness:1px;text-underline-offset:3px}a:hover{color:#123267}a:focus-visible,button:focus-visible,summary:focus-visible,.diagram-scroll:focus-visible{outline:3px solid #82a9ec;outline-offset:4px}button{font:inherit;cursor:pointer}::selection{background:#d9e8ff}.skip-link{position:fixed;top:-100px;left:20px;z-index:10;background:white;padding:12px}.skip-link:focus{top:10px}
.layout{display:grid;grid-template-columns:var(--nav) minmax(0,1fr);max-width:1580px;margin:auto}.sidebar{position:sticky;top:0;height:100vh;padding:30px 20px 20px 25px;overflow-y:auto;border-right:1px solid var(--line)}.brand{font-size:18px;font-weight:750;letter-spacing:-.6px;display:flex;align-items:center;gap:10px}.brand-icon{width:24px;height:28px;display:inline-grid;gap:3px;transform:skewY(-8deg)}.brand-icon i{background:var(--blue);border-radius:2px}.brand-icon i:nth-child(2){width:19px}.brand-icon i:nth-child(3){width:14px}.nav-label{font-size:10px;letter-spacing:1.9px;font-weight:750;color:var(--muted);margin:28px 10px 12px}.toc{display:grid;gap:3px}.toc a{display:flex;gap:10px;align-items:baseline;color:#5b6a80;text-decoration:none;font-size:12px;line-height:1.45;padding:8px 10px;border-radius:7px;border-left:2px solid transparent}.toc a span{font-size:10px;font-variant-numeric:tabular-nums;opacity:.7;flex-shrink:0}.toc a:hover{background:#eaf0f9;color:var(--blue)}.toc a[aria-current="location"]{background:#e5edf9;color:#214d91;border-left-color:var(--blue);font-weight:650}.sidebar-footer{font-size:11px;color:var(--muted);padding:22px 10px}.sidebar-footer a{display:block;margin-top:6px}
main{min-width:0;background:var(--paper);margin:24px 28px 24px 24px;border:1px solid var(--line);border-radius:18px;box-shadow:0 8px 30px #1a335407;overflow:clip}.hero{padding:45px 54px 36px;background:linear-gradient(120deg,#f8fbff,#fff 70%);border-bottom:1px solid var(--line)}.eyebrow{font-size:11px;letter-spacing:1.8px;font-weight:750;color:var(--blue);text-transform:uppercase;margin:0 0 18px}.meta-row{display:flex;flex-wrap:wrap;gap:10px;align-items:center;font-size:12px;color:var(--muted)}.draft{display:inline-flex;align-items:center;gap:7px;background:#fff1db;color:#875315;border:1px solid #eed5aa;border-radius:50px;padding:3px 11px;font-size:11px;font-weight:700}.draft:before{content:"";width:6px;height:6px;border-radius:50%;background:#bf7e28}.hero h1{font-family:Georgia,Times New Roman,serif;font-size:clamp(34px,4.2vw,55px);font-weight:500;line-height:1.12;letter-spacing:-1.8px;margin:21px 0}.hero h1 em{color:var(--blue);font-style:normal}.lead{font-size:16px;line-height:1.8;max-width:760px;color:#53647b;margin:0}.hero-actions{display:flex;flex-wrap:wrap;gap:10px;margin:25px 0}.button{display:inline-flex;align-items:center;justify-content:center;border:1px solid #cbd7e9;color:#244f92;background:#fff;text-decoration:none;border-radius:7px;padding:8px 13px;font-size:12px;font-weight:650}.button:hover{background:#edf3fd;color:#173f7e}.pill-row{display:flex;flex-wrap:wrap;gap:8px}.pill{font-size:11px;color:#526780;padding:4px 10px;border:1px solid #dde6f1;border-radius:4px;background:#f5f8fc}.intro-note{border-left:3px solid var(--teal);background:#f0f8f5;color:#2f5c50;padding:13px 18px;margin:26px 0 0;font-size:12px;line-height:1.7}.mobile-nav{display:none}.document{padding:8px 54px 32px}.provenance{font-size:12px;color:var(--muted);border-bottom:1px solid var(--line);padding:18px 0}.provenance summary{cursor:pointer;font-weight:650}.provenance p{margin:9px 0}.proposal-section{padding:35px 0 25px;border-bottom:1px solid var(--line);scroll-margin-top:20px}.proposal-section:last-child{border-bottom:0}.section-heading{display:flex;align-items:baseline;gap:12px;margin-bottom:19px}.section-number{color:#8e9eb4;font-size:11px;letter-spacing:1px;font-weight:700;flex-shrink:0}h2{font-size:23px;letter-spacing:-.65px;line-height:1.35;font-weight:700;margin:0;flex:1}.section-link{font-size:17px;text-decoration:none;color:#95a6be;padding:0 5px}p{font-size:14px;line-height:1.85;margin:15px 0;color:#3e4f65}ul,ol{padding-left:23px;margin:17px 0}li{font-size:14px;line-height:1.85;margin:8px 0;color:#3e4f65}li::marker{color:#6d89ac}strong{font-weight:700;color:#243a56}code{font-family:Cascadia Code,Consolas,monospace;font-size:.87em;background:#edf2f8;color:#254775;padding:2px 5px;border-radius:4px;overflow-wrap:anywhere}pre{background:#17273d;color:#e6effc;border-radius:10px;padding:22px 24px;overflow:auto;font-size:12px;line-height:1.7;margin:22px 0}pre code{color:inherit;background:none;padding:0;white-space:pre;font-size:inherit}.table-scroll{overflow-x:auto;border:1px solid var(--line);border-radius:9px;margin:22px 0}table{border-collapse:collapse;width:100%;font-size:12.5px;line-height:1.75;text-align:left}th{background:#eff3f9;font-size:10px;text-transform:uppercase;letter-spacing:.9px;color:#526780;padding:13px 16px;vertical-align:top}td{padding:14px 16px;border-top:1px solid var(--line);vertical-align:top;color:#43556d}td:first-child{font-weight:600;color:#294565;width:29%}tr:nth-child(even) td{background:#fbfcfe}td a{overflow-wrap:anywhere}.architecture-figure{margin:27px 0 30px;border:1px solid #dbe4f0;border-radius:12px;background:#fbfcff;overflow:hidden}.figure-label{padding:15px 20px 0;font-size:10px;font-weight:750;letter-spacing:1px;text-transform:uppercase;color:#5d7493}.diagram-scroll{padding:14px 14px 0;overflow-x:auto}.diagram{display:block;width:100%;height:auto;min-width:620px;font-family:Segoe UI,Arial,sans-serif}.node-title{font-size:15px;font-weight:650}.node-detail{font-size:12px;fill:#4b5e77}.sequence-label{font-size:11px;fill:#48617f}figcaption{font-size:11px;color:#63758c;line-height:1.7;padding:12px 20px 17px;border-top:1px solid #e7edf6;background:#f5f8fc}.footer{padding:20px 54px 28px;background:#f8fafc;border-top:1px solid var(--line);font-size:11px;color:var(--muted);display:flex;flex-wrap:wrap;gap:10px;justify-content:space-between}.progress{position:fixed;top:0;left:0;height:3px;width:0;background:var(--teal);z-index:5}.back-top{position:fixed;bottom:20px;right:25px;background:#fff;border:1px solid #ccd8e7;border-radius:7px;padding:7px 12px;font-size:11px;color:var(--blue);text-decoration:none;box-shadow:0 3px 12px #21365310}
.schema-appendices{margin:30px 0;border:1px solid var(--line);border-radius:9px;padding:0 18px;background:#fbfcfe}.schema-appendices>.provenance:last-child{border-bottom:0}.schema-appendices h2{font-size:20px;margin:25px 0 12px}.schema-appendices h3{font-size:17px}.schema-appendices .large-source{max-height:650px}.schema-appendices summary{color:var(--blue);font-size:13px}.schema-appendices[open]{background:white}
@media print{.schema-appendices .large-source{max-height:none;overflow:visible}}
@media(min-width:1500px){:root{--nav:280px}.toc a{font-size:13px}.document,.hero{padding-left:65px;padding-right:65px}p,li{font-size:15px}}
@media(max-width:1150px){:root{--nav:225px}.sidebar{padding-left:15px;padding-right:12px}.toc a{font-size:11px}.document{padding:5px 30px 28px}.hero{padding:35px 30px 28px}main{margin:16px 16px 16px 14px}.footer{padding-left:30px;padding-right:30px}}
@media(max-width:850px){.layout{display:block}.sidebar{display:none}main{margin:12px;border-radius:12px}.hero{padding:28px 24px}.document{padding:0 24px 20px}.hero h1{letter-spacing:-1px}.lead{font-size:14px}.mobile-nav{display:block;margin-top:22px;border-top:1px solid var(--line);padding-top:15px;font-size:13px}.mobile-nav summary{cursor:pointer;font-weight:650}.mobile-nav .toc{margin-top:12px;grid-template-columns:1fr 1fr}.mobile-nav .toc a{font-size:11px}h2{font-size:21px}.proposal-section{padding-top:28px}.diagram{min-width:650px}.footer{padding:20px 24px}.back-top{right:23px;bottom:14px}.table-scroll{font-size:12px}table{min-width:540px}}
@media(max-width:480px){main{margin:6px}.hero,.document{padding-left:18px;padding-right:18px}.hero h1{font-size:36px}.mobile-nav .toc{grid-template-columns:1fr}.section-heading{gap:8px}.node-title{font-size:15px}}
@media(prefers-reduced-motion:reduce){html{scroll-behavior:auto}*{animation:none!important;transition:none!important}}
@media print{@page{size:A4;margin:15mm 13mm}:root{background:white;color:#000}.layout{display:block;max-width:none}.sidebar,.progress,.back-top,.hero-actions,.mobile-nav,.section-link{display:none!important}main{margin:0;border:0;box-shadow:none;overflow:visible}.hero{background:none;padding:0 0 18px}.hero h1{font-size:34px}.lead{font-size:11px}.document{padding:0}.proposal-section{padding:22px 0 10px}.section-heading{break-after:avoid}h2{font-size:19px}p,li{font-size:10px;line-height:1.65}.architecture-figure{break-inside:avoid}.diagram-scroll{overflow:visible;padding:8px}.diagram{min-width:0;width:100%}.table-scroll{overflow:visible;border-radius:0}table{font-size:9px;min-width:0}th,td{padding:7px 9px}tr{break-inside:avoid}thead{display:table-header-group}pre{white-space:pre-wrap;font-size:9px;color:#17273d;background:#f1f4f8;border:1px solid #dbe3ee}pre code{white-space:pre-wrap}.provenance{font-size:9px}.intro-note{font-size:10px}.footer{padding:15px 0;background:none;font-size:9px}a{color:inherit}figcaption{font-size:8px}.pill{font-size:9px}}
</style></head><body id="top"><a class="skip-link" href="#proposal-content">Skip to proposal</a><div class="progress" aria-hidden="true"></div>
<div class="layout"><aside class="sidebar" aria-label="Document navigation"><div class="brand"><span class="brand-icon" aria-hidden="true"><i></i><i></i><i></i></span>FlightStrips</div><div class="nav-label">Architecture proposal</div><nav class="toc" aria-label="Contents">${toc}</nav><div class="sidebar-footer">Draft for review · Decisions 1 Nov 2026<a href="architecture-proposal.md">Markdown source ↗</a><a href="../multi-airport-support.md">Requirements discovery ↗</a></div></aside>
<main id="proposal-content"><header class="hero"><p class="eyebrow">FlightStrips / Engineering design</p><div class="meta-row"><span class="draft">Draft for review</span><time datetime="2026-11-01">Decisions confirmed 1 November 2026</time><span>Not an accepted ADR</span></div>
<h1>Multi-airport architecture.<br><em>One connection. Independent airports.</em></h1>
<p class="lead">A proposal for configurable airport workflows, shared services, and controllers working across several airports—while preserving each airport’s own operational state.</p>
<div class="hero-actions"><a class="button" href="#components-and-responsibilities">Explore the architecture ↓</a><a class="button" href="architecture-proposal.md">Read Markdown source</a><button class="button print-button" type="button">Print / save PDF</button></div>
<div class="pill-row"><span class="pill">First target: EKBI + EKYT / RADIS</span><span class="pill">ENGM informs the workflow model</span><span class="pill">Existing EKCH behavior preserved</span></div>
<div class="intro-note"><strong>Delivery decisions confirmed.</strong> <a href="#confirmed-scope-and-implementation-readiness">Ready to plan implementation epics</a>: current RADIS features, existing EKCH execution, PDC/AMAN disabled at EKBI/EKYT, and a coordinated offline upgrade. Airport values remain activation inputs.</div>
<details class="mobile-nav"><summary>Browse the ${sections.length} sections</summary><nav class="toc" aria-label="Mobile contents">${toc}</nav></details></header>
<article class="document"><details class="provenance"><summary>Document status, source, and references</summary>${markdown.parser(preamble)}<p>This HTML edition contains the full proposal and explanatory inline SVG diagrams. Markdown remains the editable source.</p></details>${contents}</article>
<footer class="footer"><span>FlightStrips · Multi-airport architecture · Draft</span><span>Generated from <a href="architecture-proposal.md">Markdown</a> · <a href="render-architecture.mjs">Renderer source</a></span></footer></main></div><a class="back-top" href="#top">↑ Top</a>
<script>
document.querySelectorAll('table').forEach(table=>{const wrapper=document.createElement('div');wrapper.className='table-scroll';wrapper.tabIndex=0;wrapper.setAttribute('role','region');wrapper.setAttribute('aria-label',table.closest('section').querySelector('h2').textContent+' table');table.before(wrapper);wrapper.append(table)});
document.querySelector('.print-button').addEventListener('click',()=>window.print());
const sections=[...document.querySelectorAll('.proposal-section')];const links=[...document.querySelectorAll('.toc a')];let scheduled=false;
function updateReadingPosition(){const y=window.scrollY;const max=document.documentElement.scrollHeight-window.innerHeight;document.querySelector('.progress').style.width=(max?100*y/max:0)+'%';let current=sections[0]?.id;for(const section of sections){if(section.getBoundingClientRect().top<=130)current=section.id;else break}links.forEach(link=>{if(link.hash==='#'+current)link.setAttribute('aria-current','location');else link.removeAttribute('aria-current')});scheduled=false}
addEventListener('scroll',()=>{if(!scheduled){requestAnimationFrame(updateReadingPosition);scheduled=true}},{passive:true});addEventListener('resize',updateReadingPosition);updateReadingPosition();
</script></body></html>`;

await writeFile(path.join(directory, 'architecture-proposal.html'), html.replace(/\r\n/g, '\n'), 'utf8');
console.log(`Rendered ${sections.length} sections and ${Object.keys(figures).length} inline diagrams from Markdown.`);
console.log(`Source SHA-256: ${sourceHash}`);
