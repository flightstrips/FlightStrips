import type { CSSProperties, ReactNode } from "react";

/**
 * 1:1 replicas of the strips the board actually renders.
 *
 * Not the real components — those need the websocket store, and this folder has
 * to stay liftable into a standalone public site. Instead the geometry, colours,
 * fonts and flex proportions are copied verbatim from:
 *
 *   ClearedSpecimen ← src/components/strip/ClxClearedStrip.tsx  (+ SIBox.tsx)
 *   ArrivalSpecimen ← src/components/strip/ApnArrStrip.tsx      (+ SIBox.tsx)
 *
 * The product sizes type in `vw` because a strip is a fraction of a bay column.
 * Here there is no bay, so the same ratios are expressed against the strip's own
 * height: at the reference 1920x1080 board, FULL_H (4.72dvh) is ~51px, which
 * makes 1.25vw ≈ 0.47h, 1.15vw ≈ 0.43h, 1.04vw ≈ 0.39h, 0.73vw ≈ 0.275h,
 * 0.63vw ≈ 0.235h, 0.57vw ≈ 0.215h, 0.42vw ≈ 0.157h and 0.21vw ≈ 0.078h.
 */

// ── Tokens copied from src/index.css and src/components/strip/shared.tsx ──────
const FONT = "'Arial', sans-serif";
const FRAME_DEP = "#18BCBC"; // --color-strip-frame
const FRAME_ARR = "#FFD100"; // --color-cell-border-arr
const BG_DEP = "#bef5ef"; //    --color-strip-dep-bg
const BG_ARR = "#fff28e"; //    --color-strip-arr-bg
const EDGE_OUTER = "#CECECE";
const EDGE_INNER = "#CCCCCC";
const SHADOW = "#2F2F2F"; //    --color-strip-shadow
const SI_ASSUMED = "#F0F0F0";
const SI_CONCERNED = "#FA02FC";
const SI_UNCONCERNED = "#DD6A12";
const BTN_ORANGE = "#DD6A12";
const SI_LABEL = "#8F8F8F";
const CDM_GREEN = "#00FF26";
const CTOT_YELLOW = "#F3EA1F";

/** src: SIBox.tsx — the six ownership backgrounds it can paint. */
export type SIState =
  | "assumed" //     you own it
  | "concerned" //   you are next
  | "unconcerned" // neither
  | "released" //    you held it and passed it on
  | "sending" //     transfer out, pending acceptance
  | "receiving"; //  transfer in, pending your acceptance

function siBackground(state: SIState): string {
  switch (state) {
    case "sending":
      return `linear-gradient(to right, ${SI_ASSUMED} 50%, ${BTN_ORANGE} 50%)`;
    case "receiving":
      return `linear-gradient(to right, ${SI_CONCERNED} 50%, ${SI_ASSUMED} 50%)`;
    case "assumed":
      return SI_ASSUMED;
    case "released":
      return BTN_ORANGE;
    case "concerned":
      return SI_CONCERNED;
    default:
      return SI_UNCONCERNED;
  }
}

type Common = {
  /** Maximum strip height in px; below that it scales with its own width.
      The product uses FULL_H = 4.72dvh, which is ~51px on a 1080 board. */
  height?: number;
  si?: SIState;
  /** Next-controller identifier shown inside the SI box. */
  nextLabel?: string;
  className?: string;
};

/** Outer frame: src/components/strip/shared.tsx getFramedStripStyle(). */
function frame(height: number, frameColor: string): CSSProperties {
  return {
    // `.fsl-strip` turns this into min(height, 11.5cqw) so the strip scales
    // as a whole instead of squeezing its cells.
    ["--fsl-strip-max" as string]: `${height}px`,
    backgroundColor: frameColor,
    boxSizing: "border-box",
    padding: 3,
    border: `1px solid ${EDGE_OUTER}`,
    boxShadow: `inset 0 0 0 1px ${EDGE_INNER}, 2px 0 0 0 ${SHADOW}, 0 -2px 0 0 ${SHADOW}`,
  };
}

function SIBoxSpecimen({
  flexGrow,
  state,
  label,
  frameColor,
}: {
  flexGrow: number;
  state: SIState;
  label?: string;
  frameColor: string;
}) {
  return (
    <div
      className="flex items-center justify-center font-bold"
      style={{
        flex: `${flexGrow} 0 0%`,
        height: "100%",
        minWidth: 0,
        background: siBackground(state),
        boxSizing: "border-box",
        borderRight: `2px solid ${frameColor}`,
        fontFamily: FONT,
        fontSize: "calc(0.43 * var(--sh))",
        color: SI_LABEL,
      }}
    >
      {label}
    </div>
  );
}

/** A stacked half-cell row with the label left and the value right. */
function TimeRow({
  label,
  value,
  background,
  color,
  borderBottom,
}: {
  label: string;
  value: ReactNode;
  background?: string;
  color?: string;
  borderBottom?: string;
}) {
  return (
    <div
      className="flex items-center justify-between overflow-hidden"
      style={{
        height: "50%",
        paddingInline: "calc(0.078 * var(--sh))",
        fontFamily: FONT,
        fontSize: "calc(0.275 * var(--sh))",
        backgroundColor: background,
        color,
        borderBottom: borderBottom ?? "2px solid transparent",
        boxSizing: "border-box",
      }}
    >
      <span className="shrink-0">{label}</span>
      <span>{value}</span>
    </div>
  );
}

export type ClearedSpecimenProps = Common & {
  callsign?: string;
  destination?: string;
  stand?: string;
  eobt?: string;
  ctot?: string;
  tobt?: string;
  tsat?: string;
  /** Paint TSAT green the way useCDMColors does once the slot is issued. */
  tsatIssued?: boolean;
};

/**
 * The CLX cleared strip. src: src/components/strip/ClxClearedStrip.tsx
 * Flex proportions: SI 8.44 | callsign 26.67 | dest+stand 13.33 | times 40.
 */
export function ClearedSpecimen({
  height = 52,
  si = "assumed",
  nextLabel,
  className = "",
  callsign = "SAS1462",
  destination = "ESSA",
  stand = "B19",
  eobt = "1515",
  ctot,
  tobt = "1518",
  tsat = "1524",
  tsatIssued = true,
}: ClearedSpecimenProps) {
  const cell = `2px solid ${FRAME_DEP}`;

  return (
    <div className={`fsl-strip select-none ${className}`} style={frame(height, FRAME_DEP)}>
      <div
        className="flex text-black"
        style={{ height: "100%", overflow: "hidden", backgroundColor: BG_DEP }}
      >
        <SIBoxSpecimen
          flexGrow={8.44}
          state={si}
          label={nextLabel}
          frameColor={FRAME_DEP}
        />

        {/* Callsign — 2/3 of the left half */}
        <div
          className="flex items-center justify-start overflow-hidden"
          style={{
            flex: "26.667 0 0%",
            height: "100%",
            minWidth: 0,
            borderRight: cell,
            boxSizing: "border-box",
            fontFamily: FONT,
            fontWeight: "bold",
            fontSize: "calc(0.47 * var(--sh))",
            paddingLeft: "calc(0.078 * var(--sh))",
          }}
        >
          <span className="w-full truncate">{callsign}</span>
        </div>

        {/* Destination over stand — 1/3 of the left half, no divider */}
        <div
          className="flex flex-col overflow-hidden"
          style={{
            flex: "13.333 0 0%",
            height: "100%",
            minWidth: 0,
            borderRight: cell,
            boxSizing: "border-box",
            fontFamily: FONT,
            fontWeight: "bold",
            fontSize: "calc(0.275 * var(--sh))",
          }}
        >
          <div className="flex items-center justify-center overflow-hidden" style={{ height: "50%" }}>
            {destination}
          </div>
          <div className="flex items-center justify-center overflow-hidden" style={{ height: "50%" }}>
            {stand}
          </div>
        </div>

        {/* EOBT/CTOT beside TOBT/TSAT */}
        <div className="flex flex-row overflow-hidden" style={{ flex: "40 0 0%", height: "100%", minWidth: 0 }}>
          <div
            className="flex flex-col overflow-hidden"
            style={{ flex: "1 0 0%", height: "100%", minWidth: 0, borderRight: cell, boxSizing: "border-box" }}
          >
            <TimeRow
              label="EOBT"
              value={eobt}
              borderBottom={ctot ? cell : undefined}
            />
            <TimeRow
              label={ctot ? "CTOT" : ""}
              value={ctot ?? ""}
              background={ctot ? CTOT_YELLOW : undefined}
            />
          </div>

          <div className="flex flex-col" style={{ flex: "1 0 0%", height: "100%" }}>
            <TimeRow label="TOBT" value={tobt} borderBottom={cell} />
            <TimeRow
              label="TSAT"
              value={tsat}
              background={tsatIssued ? CDM_GREEN : undefined}
            />
          </div>
        </div>
      </div>
    </div>
  );
}

export type ArrivalSpecimenProps = Common & {
  callsign?: string;
  nextFreq?: string;
  aircraftType?: string;
  registration?: string;
  runway?: string;
  taxiway?: string;
  stand?: string;
};

/**
 * The apron arrival strip. src: src/components/strip/ApnArrStrip.tsx
 * Flex proportions: SI 40 | callsign 120 | type 80 | rwy 54 | twy 54 | stand 80.
 * Rows split 66.67% / 33.33%.
 */
export function ArrivalSpecimen({
  height = 52,
  si = "concerned",
  nextLabel,
  className = "",
  callsign = "DLH820",
  nextFreq = "119.900",
  aircraftType = "A21N",
  registration = "D-AIEA",
  runway = "22L",
  taxiway = "M8",
  stand = "A07",
}: ArrivalSpecimenProps) {
  const cell = `2px solid ${FRAME_ARR}`;
  const TOP = "66.6667%";
  const BOT = "33.3333%";

  return (
    <div className={`fsl-strip select-none ${className}`} style={frame(height, FRAME_ARR)}>
      <div
        className="flex text-black"
        style={{ height: "100%", overflow: "hidden", backgroundColor: BG_ARR }}
      >
        <SIBoxSpecimen
          flexGrow={40}
          state={si}
          label={nextLabel}
          frameColor={FRAME_ARR}
        />

        {/* Callsign over next frequency */}
        <div
          className="flex min-w-0 flex-col"
          style={{ flexGrow: 120, flexBasis: 0, height: "100%", borderRight: cell, boxSizing: "border-box" }}
        >
          <div className="flex items-center" style={{ height: TOP, paddingLeft: "calc(0.157 * var(--sh))" }}>
            <span
              className="w-full truncate"
              style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "calc(0.39 * var(--sh))" }}
            >
              {callsign}
            </span>
          </div>
          <div className="flex items-center overflow-hidden" style={{ height: BOT, paddingLeft: "calc(0.157 * var(--sh))" }}>
            <span
              className="w-full truncate"
              style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "calc(0.215 * var(--sh))" }}
            >
              {nextFreq}
            </span>
          </div>
        </div>

        {/* Aircraft type over registration */}
        <div
          className="flex min-w-0 flex-col"
          style={{ flexGrow: 80, flexBasis: 0, height: "100%", borderRight: cell, boxSizing: "border-box" }}
        >
          <div className="flex items-center justify-center" style={{ height: TOP }}>
            <span
              className="truncate"
              style={{ fontFamily: FONT, fontWeight: 600, fontSize: "calc(0.235 * var(--sh))", paddingInline: "calc(0.078 * var(--sh))" }}
            >
              {aircraftType}
            </span>
          </div>
          <div className="flex items-center justify-center overflow-hidden" style={{ height: BOT }}>
            <span
              className="truncate"
              style={{ fontFamily: FONT, fontSize: "calc(0.235 * var(--sh))", paddingInline: "calc(0.078 * var(--sh))" }}
            >
              {registration}
            </span>
          </div>
        </div>

        {[
          { value: runway, grow: 54, border: true },
          { value: taxiway, grow: 54, border: true },
          { value: stand, grow: 80, border: false },
        ].map((column) => (
          <div
            key={column.value}
            className="flex min-w-0 flex-col overflow-hidden"
            style={{
              flexGrow: column.grow,
              flexBasis: 0,
              height: "100%",
              boxSizing: "border-box",
              ...(column.border ? { borderRight: cell } : {}),
            }}
          >
            <div className="flex items-center justify-center" style={{ height: TOP }}>
              <span
                className="truncate"
                style={{ fontFamily: FONT, fontWeight: "bold", fontSize: "calc(0.39 * var(--sh))" }}
              >
                {column.value}
              </span>
            </div>
            <div style={{ height: BOT }} />
          </div>
        ))}
      </div>
    </div>
  );
}
