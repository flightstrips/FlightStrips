/**
 * How FlightStrips sits between its users and the services it talks to.
 *
 * Every node and edge here is something that exists:
 *   - controller web app + EuroScope plugin over websocket  (backend/internal/websocket, internal/euroscope)
 *   - pilot web pages over HTTPS                            (backend/internal/pilot, internal/efb)
 *   - VATSIM SSO identity matched by CID                    (backend/internal/services/authentication.go)
 *   - VATSIM data feed and AFV ATIS                         (backend/internal/vatsim, internal/metar)
 *   - Hoppie ACARS for PDC                                  (backend/internal/pdc/hoppie.go)
 *   - ECFMP flow measures                                   (backend/internal/ecfmp/client.go)
 *   - PostgreSQL for session and strip state                (backend/internal/repository, migrations)
 */

const NODE_FILL = "var(--fsl-surface)";
const NODE_LINE = "var(--fsl-line-strong)";
const INK = "var(--fsl-ink)";
const MUTED = "var(--fsl-ink-muted)";
const BRAND = "var(--fsl-brand-ink)";

type BoxProps = {
  x: number;
  y: number;
  w: number;
  h: number;
  title: string;
  sub?: string;
  accent?: boolean;
};

function Box({ x, y, w, h, title, sub, accent }: BoxProps) {
  return (
    <g>
      <rect
        x={x}
        y={y}
        width={w}
        height={h}
        fill={NODE_FILL}
        stroke={accent ? BRAND : NODE_LINE}
        strokeWidth={1}
      />
      <text
        x={x + 16}
        y={sub ? y + h / 2 - 4 : y + h / 2 + 5}
        fill={INK}
        fontSize={15}
        fontWeight={600}
        letterSpacing="-0.01em"
      >
        {title}
      </text>
      {sub ? (
        <text x={x + 16} y={y + h / 2 + 16} fill={MUTED} fontSize={11.5} letterSpacing="0.04em">
          {sub}
        </text>
      ) : null}
    </g>
  );
}

/** Horizontal connector. `both` draws an arrowhead at each end. */
function Edge({
  x1,
  x2,
  y,
  label,
  both = false,
}: {
  x1: number;
  x2: number;
  y: number;
  label?: string;
  both?: boolean;
}) {
  return (
    <g>
      <line
        x1={x1}
        y1={y}
        x2={x2}
        y2={y}
        stroke={NODE_LINE}
        strokeWidth={1}
        markerEnd="url(#fsl-arrow)"
        {...(both ? { markerStart: "url(#fsl-arrow-back)" } : {})}
      />
      {label ? (
        <text
          x={(x1 + x2) / 2}
          y={y - 9}
          fill={MUTED}
          fontSize={10.5}
          letterSpacing="0.14em"
          textAnchor="middle"
        >
          {label}
        </text>
      ) : null}
    </g>
  );
}

const SERVER_ROWS = [
  "Session and shared strip state",
  "Ownership, handoffs and routes",
  "A-CDM: TOBT / TSAT / TTOT / CTOT",
  "Arrival manager and sequencing",
  "Validation and airport rules",
];

export function SystemDiagram() {
  return (
    <figure className="m-0">
      <div className="overflow-x-auto" tabIndex={0} aria-label="System diagram, scrollable">
        <svg
          viewBox="0 0 980 548"
          role="img"
          aria-labelledby="fsl-diagram-title fsl-diagram-desc"
          className="block h-auto w-full min-w-[860px]"
          style={{ fontFamily: "var(--fsl-font-mono)" }}
        >
          <title id="fsl-diagram-title">How FlightStrips connects controllers, pilots and services</title>
          <desc id="fsl-diagram-desc">
            Controllers use the web app and the EuroScope plugin; pilots use the pilot pages. All three
            connect to the FlightStrips server, which holds session and strip state, ownership and
            handoffs, A-CDM times, the arrival manager and validation rules, and stores them in
            PostgreSQL. The server calls out to VATSIM single sign-on, the VATSIM data and ATIS feed,
            Hoppie ACARS for pre-departure clearances, and ECFMP for flow measures.
          </desc>

          <defs>
            <marker id="fsl-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto">
              <path d="M0,0 L10,5 L0,10 z" fill={NODE_LINE} />
            </marker>
            <marker id="fsl-arrow-back" viewBox="0 0 10 10" refX="1" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
              <path d="M0,0 L10,5 L0,10 z" fill={NODE_LINE} />
            </marker>
          </defs>

          {/* ── Column headings ─────────────────────────────── */}
          <text x={24} y={30} fill={MUTED} fontSize={10.5} letterSpacing="0.22em">
            USERS AND CLIENTS
          </text>
          <text x={316} y={30} fill={MUTED} fontSize={10.5} letterSpacing="0.22em">
            FLIGHTSTRIPS
          </text>
          <text x={748} y={30} fill={MUTED} fontSize={10.5} letterSpacing="0.22em">
            EXTERNAL SERVICES
          </text>

          {/* ── Users and clients ───────────────────────────── */}
          <Box x={24} y={64} w={208} h={88} title="Controller" sub="Strip board in the browser" />
          <Box x={24} y={176} w={208} h={88} title="EuroScope" sub="FlightStrips plugin" />
          <Box x={24} y={288} w={208} h={88} title="Pilot" sub="Flight and EFB pages" />

          {/* ── The server ──────────────────────────────────── */}
          <rect x={316} y={40} width={348} height={400} fill="var(--fsl-raised)" stroke={BRAND} strokeWidth={1} />
          <text x={340} y={76} fill={INK} fontSize={17} fontWeight={600} letterSpacing="-0.01em">
            FlightStrips server
          </text>
          {SERVER_ROWS.map((row, index) => (
            <g key={row}>
              <rect
                x={340}
                y={104 + index * 56}
                width={300}
                height={44}
                fill={NODE_FILL}
                stroke={NODE_LINE}
                strokeWidth={1}
              />
              <text x={356} y={104 + index * 56 + 27} fill={INK} fontSize={12.5} letterSpacing="0.01em">
                {row}
              </text>
            </g>
          ))}
          <text x={340} y={418} fill={MUTED} fontSize={11}>
            One session per airport, broadcast to every client
          </text>

          {/* ── Storage ─────────────────────────────────────── */}
          <line x1={490} y1={440} x2={490} y2={462} stroke={NODE_LINE} strokeWidth={1} markerEnd="url(#fsl-arrow)" />
          <Box x={316} y={464} w={348} h={60} title="PostgreSQL" sub="Sessions, strips and history" />

          {/* ── Client edges ────────────────────────────────── */}
          <Edge x1={232} x2={316} y={108} label="WEBSOCKET" both />
          <Edge x1={232} x2={316} y={220} label="WEBSOCKET" both />
          <Edge x1={232} x2={316} y={332} label="HTTPS" both />

          {/* ── External services ───────────────────────────── */}
          <Box x={748} y={64} w={208} h={72} title="VATSIM SSO" sub="Identity, matched by CID" />
          <Box x={748} y={152} w={208} h={72} title="VATSIM feed" sub="Traffic, ATIS and METAR" />
          <Box x={748} y={240} w={208} h={72} title="Hoppie ACARS" sub="Pre-departure clearance" />
          <Box x={748} y={328} w={208} h={72} title="ECFMP" sub="Flow measures" />

          <Edge x1={664} x2={748} y={100} />
          <Edge x1={664} x2={748} y={188} />
          <Edge x1={664} x2={748} y={276} />
          <Edge x1={664} x2={748} y={364} />
        </svg>
      </div>

      <figcaption className="mt-6 max-w-2xl text-sm leading-relaxed text-[var(--fsl-ink-muted)]">
        Controllers and pilots never talk to each other directly. Every action becomes a server event,
        the server decides what the shared state now is, and each connected client receives the result.
      </figcaption>
    </figure>
  );
}
