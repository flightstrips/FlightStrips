import { useEffect, useRef, useState, type CSSProperties } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  closestCenter,
  pointerWithin,
  useDroppable,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import CommandBar from "./CommandBar";
import { FindDialog, NewFplDialog } from "./CommandDialogs";
import {
  findStandAtPosition,
  isStandBlockedByRadius,
  parseStandAssignmentFile,
  type StandAssignment,
} from "./standAssignment";

type BayId = "ACTIVE" | "RUNWAY" | "PASSIVE_TOP" | "PASSIVE_BOTTOM";
type MenuType = "RTE" | "ALT" | "NUM" | "FPL" | "RWY";
type NumField = "B5" | "B6" | "B7";
type StripType = "ARRIVAL" | "DEPARTURE";
const DELETE_BAY_ID = "DELETE_BAY";

const DEFAULT_AIRPORT = "EKYT";
const AIRPORT_OPTIONS = ["EKYT", "EKBI", "EKAH", "EKSP", "EKKA"];
const SELECTED_AIRPORT_STORAGE_KEY = "fsradis.selected-airport";
const VATSIM_DATA_URL = "https://data.vatsim.net/v3/vatsim-data.json";
const AIRPORT_CENTERS: Record<string, { latitude: number; longitude: number }> =
  {
    EKYT: { latitude: 57.086428, longitude: 9.870372 },
    EKBI: { latitude: 55.742539, longitude: 9.144579 },
    EKAH: { latitude: 56.307303, longitude: 10.626083 },
    EKSP: { latitude: 55.22189, longitude: 9.284179 },
    EKKA: { latitude: 56.31089, longitude: 9.118811 },
  };
const AIRPORT_RADIUS_NM = 150;
const METERS_PER_NAUTICAL_MILE = 1_852;
const SCHENGEN_PREFIXES = new Set([
  "EH",
  "EK",
  "EL",
  "EN",
  "EP",
  "ES",
  "ET",
  "EV",
  "EY",
  "GC",
  "LB",
  "LD",
  "LE",
  "LF",
  "LG",
  "LH",
  "LI",
  "LJ",
  "LK",
  "LM",
  "LO",
  "LP",
  "LR",
  "LS",
  "LZ",
]);

function isSchengenAirport(airport: string): boolean {
  const prefix = (airport ?? "").toUpperCase().slice(0, 2);
  return SCHENGEN_PREFIXES.has(prefix);
}

function getStoredAirport(): string {
  try {
    const storedAirport = window.localStorage
      .getItem(SELECTED_AIRPORT_STORAGE_KEY)
      ?.toUpperCase();
    return storedAirport && AIRPORT_OPTIONS.includes(storedAirport)
      ? storedAirport
      : DEFAULT_AIRPORT;
  } catch {
    return DEFAULT_AIRPORT;
  }
}

function isWithinAirportRadius(
  airport: string,
  latitude: number | undefined,
  longitude: number | undefined,
): boolean {
  const center = AIRPORT_CENTERS[airport];
  if (
    !center ||
    typeof latitude !== "number" ||
    typeof longitude !== "number"
  ) {
    return false;
  }

  const toRadians = (degrees: number) => (degrees * Math.PI) / 180;
  const latitudeDelta = toRadians(center.latitude - latitude);
  const longitudeDelta = toRadians(center.longitude - longitude);
  const haversine =
    Math.sin(latitudeDelta / 2) ** 2 +
    Math.cos(toRadians(latitude)) *
      Math.cos(toRadians(center.latitude)) *
      Math.sin(longitudeDelta / 2) ** 2;
  const distanceMeters =
    2 * 6_371_000 * Math.atan2(Math.sqrt(haversine), Math.sqrt(1 - haversine));

  return distanceMeters <= AIRPORT_RADIUS_NM * METERS_PER_NAUTICAL_MILE;
}

interface VatsimPilot {
  callsign: string;
  altitude?: number;
  latitude?: number;
  longitude?: number;
  flight_plan?: {
    departure?: string;
    arrival?: string;
    aircraft_short?: string;
    route?: string;
    altitude?: string;
    deptime?: string;
    enroute_time?: string;
    assigned_transponder?: string;
    wake_turbulence?: string;
    flight_rules?: string;
  };
}

interface VatsimDataResponse {
  pilots?: VatsimPilot[];
}

interface FlightPlanSeed {
  callsign: string;
  departure: string;
  arrival: string;
  aircraft: string;
  wakeTurbulence: string;
  route: string;
  altitude: string;
  depTime: string;
  hrsEnroute: string;
  minEnroute: string;
  assignedSquawk: string;
  registration?: string;
  assignedRunway?: string;
  sid?: string;
  rnav?: string;
  radisRemarks?: string;
  vfr?: boolean;
  flightRules?: "I" | "V";
  source: "vatsim" | "fake" | "manual" | "planned";
  latitude?: number;
  longitude?: number;
  // Live reported altitude (ft) from VATSIM, used to auto-stamp ATD; absent for manual/fake seeds.
  liveAltitudeFt?: number | null;
}

interface StripDisplayData {
  callsign: string;
  departure: string;
  arrival: string;
  aircraft: string;
  wakeTurbulence: string;
  route: string;
  firstWaypoint: string;
  firstAirwayOrSecondPoint: string;
  altitude: string;
  depTime: string;
  hrsEnroute: string;
  minEnroute: string;
  assignedSquawk: string;
  registration?: string;
  flightRules?: "I" | "V";
  stripType: StripType;
  source: "vatsim" | "fake" | "manual" | "planned";
}

interface PilotPosition {
  latitude?: number;
  longitude?: number;
}

interface BayState {
  ACTIVE: string[];
  RUNWAY: string[];
  PASSIVE_TOP: string[];
  PASSIVE_BOTTOM: string[];
}

interface StripValues {
  U2: string;
  B2: string;
  U3: string;
  B3: string;
  U4: string;
  B4: string;
  U5: string;
  B5: string;
  U6: string;
  B6: string;
  U7: string;
  B7: string;
  rea: string;
  deIce: boolean;
  arrivalRwy: string;
  rnav: string;
  radisRemarks: string;
  clearanceSquawk: string;
  vfrOverride: boolean;
}

interface MenuState {
  type: MenuType;
  stripId: string;
  field?: NumField;
  target?: "U2" | "arrivalRwy";
}

interface SortablePrototypeStripProps {
  stripId: string;
  stripType: StripType;
  displayData: StripDisplayData;
  stripStyle: CSSProperties;
  displayDate: string;
  values: StripValues;
  assignedStand: string;
  onOpenMenu: (type: MenuType, stripId: string, field?: NumField) => void;
  onRequestQnhUpdate: (stripId: string) => void;
  onToggleAta: (stripId: string) => void;
  onOpenFpl: (stripId: string) => void;
}

interface PrototypeStripSectionsProps {
  displayDate: string;
  stripId: string;
  stripType: StripType;
  displayData: StripDisplayData;
  values: StripValues;
  assignedStand: string;
  onOpenMenu: (type: MenuType, stripId: string, field?: NumField) => void;
  onRequestQnhUpdate: (stripId: string) => void;
  onToggleAta: (stripId: string) => void;
  onOpenFpl: (stripId: string) => void;
  dragHandleProps?: any;
}

interface SortableStripListProps {
  bayId: BayId;
  stripIds: string[];
  stripStyle: CSSProperties;
  displayDate: string;
  stripValues: Record<string, StripValues>;
  stripTypes: Record<string, StripType>;
  stripData: Record<string, StripDisplayData>;
  stripStandAssignments: Record<string, string>;
  onOpenMenu: (type: MenuType, stripId: string, field?: NumField) => void;
  onRequestQnhUpdate: (stripId: string) => void;
  onToggleAta: (stripId: string) => void;
  onOpenFpl: (stripId: string) => void;
  reverse?: boolean;
}

function SortableStripList({
  bayId,
  stripIds,
  stripStyle,
  displayDate,
  stripValues,
  stripTypes,
  stripData,
  stripStandAssignments,
  onOpenMenu,
  onRequestQnhUpdate,
  onToggleAta,
  onOpenFpl,
  reverse = false,
}: SortableStripListProps) {
  const { setNodeRef } = useDroppable({ id: bayId });

  return (
    <div
      ref={setNodeRef}
      className={`strip-list strip-drop-zone ${reverse ? "reverse-order" : ""}`}
    >
      <SortableContext items={stripIds} strategy={verticalListSortingStrategy}>
        {stripIds.map((stripId) => (
          <SortablePrototypeStrip
            key={stripId}
            stripId={stripId}
            stripType={stripTypes[stripId] ?? "ARRIVAL"}
            displayData={
              stripData[stripId] ?? {
                callsign: "N/A",
                departure: DEFAULT_AIRPORT,
                arrival: "EKCH",
                aircraft: "A320",
                wakeTurbulence: "M",
                route: "",
                firstWaypoint: "",
                firstAirwayOrSecondPoint: "",
                altitude: "",
                depTime: "",
                hrsEnroute: "",
                minEnroute: "",
                assignedSquawk: "",
                stripType: stripTypes[stripId] ?? "ARRIVAL",
                source: "fake",
              }
            }
            stripStyle={stripStyle}
            displayDate={displayDate}
            values={
              stripValues[stripId] ?? {
                U2: "",
                B2: "",
                U3: "",
                B3: "",
                U4: "",
                B4: "",
                U5: "0000",
                B5: "",
                U6: "0000",
                B6: "",
                U7: "0000",
                B7: "",
                rea: "",
                deIce: false,
                arrivalRwy: "",
              }
            }
            assignedStand={stripStandAssignments[stripId] ?? ""}
            onOpenMenu={onOpenMenu}
            onRequestQnhUpdate={onRequestQnhUpdate}
            onToggleAta={onToggleAta}
            onOpenFpl={onOpenFpl}
          />
        ))}
      </SortableContext>
    </div>
  );
}

interface RunwayMenuRow {
  left: string;
  right: string;
}

interface RunwayMenuConfig {
  rows: RunwayMenuRow[];
  topLineScale: number;
  bottomLineScale: number;
}

function getRunwayMenuConfig(airport: string): RunwayMenuConfig {
  switch (airport) {
    case "EKKA":
      return {
        rows: [
          { left: "09L", right: "27R" },
          { left: "09R", right: "27L" },
        ],
        topLineScale: 0.5,
        bottomLineScale: 1,
      };
    case "EKSP":
      return {
        rows: [
          { left: "10L", right: "28R" },
          { left: "10R", right: "28L" },
        ],
        topLineScale: 1,
        bottomLineScale: 0.5,
      };
    case "EKAH":
      return {
        rows: [
          { left: "10L", right: "28R" },
          { left: "10R", right: "28L" },
        ],
        topLineScale: 0.5,
        bottomLineScale: 1,
      };
    case "EKBI":
      return {
        rows: [{ left: "09", right: "27" }],
        topLineScale: 1,
        bottomLineScale: 1,
      };
    default:
      return {
        rows: [
          { left: "08L", right: "26R" },
          { left: "08R", right: "26L" },
        ],
        topLineScale: 1,
        bottomLineScale: 1,
      };
  }
}

interface PointMenuConfig {
  left: (string | null)[];
  right: (string | null)[];
  center: string | null;
}

function getPointMenuConfig(airport: string): PointMenuConfig {
  switch (airport) {
    case "EKBI":
      return {
        left: ["EPARA", "GELBA", "GIVNA"],
        right: ["ODNEV", "LOKSA", "UVINA"],
        center: null,
      };
    case "EKAH":
      return {
        left: ["ALINI", "MOMZE", "GINUB"],
        right: ["BADUT", "URUPA", "DESOM"],
        center: "TL",
      };
    case "EKKA":
      return {
        left: ["BAMRU", "REVBO", "EBISO"],
        right: ["GIROG", "RIKSU", "LIRGO"],
        center: null,
      };
    case "EKSP":
      return {
        left: [null, "TISET", null],
        right: [null, "DINUT", null],
        center: "VO",
      };
    default:
      return {
        left: ["TOSVI", "GIPUG", "KUDEV"],
        right: ["UPZIW", "BAKIT", "EWTIQ"],
        center: "AAL",
      };
  }
}

function MenuDialog({
  menuState,
  value,
  onValueChange,
  onEsc,
  onOk,
  onSubmitValue,
  onPreset,
  airport,
}: {
  menuState: MenuState | null;
  value: string;
  onValueChange: (next: string) => void;
  onEsc: () => void;
  onOk: () => void;
  onSubmitValue: (nextValue: string) => void;
  onPreset: (preset: string) => void;
  airport: string;
}) {
  const [pointDialogOpen, setPointDialogOpen] = useState(false);

  useEffect(() => {
    if (!menuState || menuState.type !== "RTE") {
      setPointDialogOpen(false);
    }
  }, [menuState]);

  if (!menuState) {
    return null;
  }

  const isNum = menuState.type === "NUM";
  const isAlt = menuState.type === "ALT";
  const isRte = menuState.type === "RTE";
  const isRwy = menuState.type === "RWY";

  const rteHeadingOptions = ["110", "230", "050", "290", "350", "170"];
  const runwayMenuConfig = getRunwayMenuConfig(airport);
  const runwayPairs = runwayMenuConfig.rows;
  const altOptions = [
    "FL130",
    "FL120",
    "FL70",
    "FL50",
    "4000",
    "3000",
    "2300",
    "2000",
    "1500",
  ];
  const pointOptions = getPointMenuConfig(airport);

  function handlePointSelect(point: string) {
    onSubmitValue(point);
    setPointDialogOpen(false);
  }

  function handleRunwaySelect(runway: string) {
    onSubmitValue(runway);
  }

  if (isRte && pointDialogOpen) {
    return (
      <div
        className="menu-overlay"
        role="dialog"
        aria-modal="true"
        aria-label="Point Dialogue"
      >
        <div className="point-menu-frame">
          <div className="point-menu-inner">
            <div className="point-menu-row point-menu-row-top">
              {pointOptions.left[0] && (
                <button
                  className="point-menu-box"
                  onClick={() => handlePointSelect(pointOptions.left[0]!)}
                >
                  {pointOptions.left[0]}
                </button>
              )}
              {pointOptions.right[0] && (
                <button
                  className="point-menu-box"
                  onClick={() => handlePointSelect(pointOptions.right[0]!)}
                >
                  {pointOptions.right[0]}
                </button>
              )}
            </div>

            <div className="point-menu-center-row" aria-hidden>
              <div className="point-menu-line-group point-menu-line-group-left">
                <div className="point-menu-line" />
                <div className="point-menu-line" />
              </div>
              <div className="point-menu-line-main" />
              {pointOptions.center && (
                <button
                  className="point-menu-box point-menu-box-center"
                  onClick={() => handlePointSelect(pointOptions.center!)}
                >
                  {pointOptions.center}
                </button>
              )}
              <div className="point-menu-line-group point-menu-line-group-right">
                <div className="point-menu-line" />
                <div className="point-menu-line" />
              </div>
            </div>

            <div className="point-menu-row point-menu-row-mid">
              {pointOptions.left[1] && (
                <button
                  className="point-menu-box"
                  onClick={() => handlePointSelect(pointOptions.left[1]!)}
                >
                  {pointOptions.left[1]}
                </button>
              )}
              {pointOptions.right[1] && (
                <button
                  className="point-menu-box"
                  onClick={() => handlePointSelect(pointOptions.right[1]!)}
                >
                  {pointOptions.right[1]}
                </button>
              )}
            </div>

            <div className="point-menu-row point-menu-row-low">
              {pointOptions.left[2] && (
                <button
                  className="point-menu-box"
                  onClick={() => handlePointSelect(pointOptions.left[2]!)}
                >
                  {pointOptions.left[2]}
                </button>
              )}
              {pointOptions.right[2] && (
                <button
                  className="point-menu-box"
                  onClick={() => handlePointSelect(pointOptions.right[2]!)}
                >
                  {pointOptions.right[2]}
                </button>
              )}
            </div>

            <button className="point-menu-esc" onClick={onEsc}>
              ESC
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (isRwy) {
    return (
      <div
        className="menu-overlay"
        role="dialog"
        aria-modal="true"
        aria-label="RWY Menu"
      >
        <div className="menu-panel menu-panel-rwy">
          <div className="menu-title">RWY MENU</div>

          <div className="menu-grid menu-grid-rwy">
            {runwayPairs.map((pair, index) => (
              <div
                key={`${pair.left}-${pair.right}`}
                className={`menu-rwy-row menu-rwy-row-${index + 1}`}
              >
                <button
                  className="menu-rwy-box"
                  onClick={() => handleRunwaySelect(pair.left)}
                >
                  {pair.left}
                </button>
                <div
                  className="menu-rwy-line"
                  aria-hidden
                  style={{
                    height: `${(index === 0 ? runwayMenuConfig.topLineScale : runwayMenuConfig.bottomLineScale) * 10}px`,
                  }}
                />
                <button
                  className="menu-rwy-box"
                  onClick={() => handleRunwaySelect(pair.right)}
                >
                  {pair.right}
                </button>
              </div>
            ))}
          </div>

          <div className="menu-actions menu-actions-rwy">
            <button className="menu-action menu-action-esc" onClick={onEsc}>
              ESC
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="menu-overlay" role="dialog" aria-modal="true">
      <div className={`menu-panel ${isRte ? "menu-panel-rte" : ""}`}>
        {!isRte && <div className="menu-title">{menuState.type} MENU</div>}

        {isRte ? (
          <>
            <div className="menu-grid menu-grid-rte">
              {rteHeadingOptions.map((option) => (
                <button
                  key={option}
                  className={`menu-key ${value === option ? "is-selected" : ""}`}
                  onClick={() => onPreset(option)}
                >
                  {option}
                </button>
              ))}
            </div>

            <button
              className="menu-key menu-key-point"
              onClick={() => setPointDialogOpen(true)}
            >
              POINT
            </button>
          </>
        ) : null}

        {isAlt ? (
          <div className="menu-grid menu-grid-alt">
            {altOptions.map((option) => (
              <button
                key={option}
                className={`menu-key ${value === option ? "is-selected" : ""}`}
                onClick={() => onPreset(option)}
              >
                {option}
              </button>
            ))}
          </div>
        ) : null}

        {isNum ? (
          <div className="menu-grid menu-grid-num">
            {["7", "8", "9", "4", "5", "6", "1", "2", "3", ".", "0", "/"].map(
              (keyValue) => (
                <button
                  key={keyValue}
                  className="menu-key"
                  onClick={() => onValueChange(`${value}${keyValue}`)}
                >
                  {keyValue}
                </button>
              ),
            )}
          </div>
        ) : null}

        {menuState.type === "FPL" ? (
          <div className="menu-fpl-placeholder">
            Flightplan popup will be integrated from FlightStrips.
          </div>
        ) : (
          <input
            className={`menu-display ${isAlt ? "menu-display-alt" : ""}`}
            value={value}
            onChange={(event) =>
              onValueChange(event.target.value.toUpperCase())
            }
            autoFocus
          />
        )}

        <div className="menu-actions">
          <button className="menu-action menu-action-esc" onClick={onEsc}>
            ESC
          </button>
          <button className="menu-action menu-action-ok" onClick={onOk}>
            OK
          </button>
        </div>
      </div>
    </div>
  );
}

function PrototypeStripSections({
  displayDate,
  stripId,
  stripType,
  displayData,
  values,
  assignedStand,
  onOpenMenu,
  onRequestQnhUpdate,
  onToggleAta,
  onOpenFpl,
  dragHandleProps,
}: PrototypeStripSectionsProps) {
  return (
    <>
      <section
        className="strip-box box-1 strip-drag-handle"
        {...(dragHandleProps ?? {})}
        aria-label="Drag strip handle"
      >
        <div className="box-1-row box-1-row-1">{displayData.callsign}</div>

        <div className="box-1-row box-1-row-2">
          <span>{displayData.departure}</span>
          <div
            className="box-1-middle-stack"
            aria-label="First two route points"
          >
            <span className="box-1-middle-stack-line">
              {displayData.firstWaypoint}
            </span>
            <span className="box-1-middle-stack-line">
              {displayData.firstAirwayOrSecondPoint}
            </span>
          </div>
          <span>{displayData.arrival}</span>
        </div>

        <div className="box-1-row box-1-row-3">
          <span>{displayData.aircraft}</span>
          <span>{displayData.wakeTurbulence}</span>
          <span>{displayData.assignedSquawk}</span>
          <span aria-hidden />
        </div>
      </section>

      <section className="strip-box box-2 split-borders">
        <button
          className="box-cell box-cell-bold box-cell-clickable"
          onClick={() => onOpenMenu("RWY", stripId)}
          aria-label="Open RWY MENU"
        >
          {values.U2}
        </button>
        <button
          className="box-cell box-cell-bold"
          onClick={() => onRequestQnhUpdate(stripId)}
        >
          {values.B2 || "QNH"}
        </button>
      </section>

      <section className="strip-box box-3">
        <button
          className="box-cell box-cell-left box-cell-clickable"
          onClick={() => onOpenMenu("RTE", stripId)}
          aria-label="Open RTE MENU top"
        >
          {values.U3}
        </button>
        <button
          className="box-cell box-cell-left box-cell-clickable"
          onClick={() => onOpenMenu("RTE", stripId)}
          aria-label="Open RTE MENU bottom"
        >
          {values.B3}
        </button>
      </section>

      <section className="strip-box box-4">
        <button
          className="box-cell box-cell-bold box-cell-left box-cell-top-left box-cell-altitude-filed"
          aria-label="Filed flight level"
        >
          {values.U4}
        </button>
        <button
          className="box-cell box-cell-clickable box-cell-altitude-entered"
          onClick={() => onOpenMenu("ALT", stripId)}
          aria-label="Open ALT MENU bottom"
        >
          <span>
            {values.vfrOverride ? "V" : (displayData.flightRules ?? "")}
          </span>
          <span>{values.B4}</span>
        </button>
      </section>

      <section className="strip-box box-5 split-borders">
        <div
          className="labeled-cell labeled-cell-static"
          aria-label="EOBT static"
        >
          <span className="labeled-cell-header">EOBT</span>
          <span className="labeled-cell-value">{values.U5}</span>
        </div>
        <button
          className="labeled-cell box-cell-clickable"
          onClick={() => onOpenMenu("NUM", stripId, "B5")}
          aria-label="Open NUM MENU ATD"
        >
          <span className="labeled-cell-header">ATD</span>
          <span className="labeled-cell-value">{values.B5}</span>
        </button>
      </section>

      <section className="strip-box box-6 split-borders">
        <div
          className="labeled-cell labeled-cell-static"
          aria-label="EET static"
        >
          <span className="labeled-cell-header">EET</span>
          <span className="labeled-cell-value">{values.U6}</span>
        </div>
        <button
          className="labeled-cell box-cell-clickable"
          onClick={() => onOpenMenu("NUM", stripId, "B6")}
          aria-label="Open NUM MENU END/POB"
        >
          <span className="labeled-cell-header">END/POB</span>
          <span className="labeled-cell-value">{values.B6}</span>
        </button>
      </section>

      <section className="strip-box box-7 split-borders">
        <div
          className="labeled-cell labeled-cell-static"
          aria-label="ETA static"
        >
          <span className="labeled-cell-header">ETA</span>
          <span className="labeled-cell-value">{values.U7}</span>
        </div>
        <button
          className="labeled-cell box-cell-clickable"
          onClick={() => onToggleAta(stripId)}
          aria-label="Toggle ATA time"
        >
          <span className="labeled-cell-header">ATA</span>
          <span className="labeled-cell-value">{values.B7}</span>
        </button>
      </section>

      <button
        className="strip-box box-8 box-cell-clickable"
        onClick={() => onOpenFpl(stripId)}
        aria-label="Open FPL"
      >
        <div className="box-8-top">
          <div
            className={`box-8-top-right-icon ${stripType === "DEPARTURE" ? "is-departure" : "is-arrival"}`}
            aria-hidden
          >
            <svg
              viewBox="0 0 64 64"
              role="img"
              aria-label={
                stripType === "DEPARTURE" ? "Departure icon" : "Arrival icon"
              }
              focusable="false"
            >
              <path
                d="M8 32h30l-8-8 6-6 18 18-18 18-6-6 8-8H8z"
                fill="currentColor"
              />
            </svg>
          </div>
        </div>
        <div className="box-8-bottom box-8-bottom-left">
          <span className="labeled-cell-header">stand</span>
          <span className="labeled-cell-value">{assignedStand ?? ""}</span>
        </div>
        <div className="box-8-bottom box-8-bottom-middle" />
        <div className="box-8-bottom box-8-bottom-right">
          <span className="ap-text">
            {(stripType === "ARRIVAL"
              ? displayData.arrival
              : displayData.departure
            ).slice(-2)}
          </span>
        </div>
      </button>

      <section className="strip-box box-9" aria-label="Date">
        <span className="date-rotated">{displayDate}</span>
      </section>
    </>
  );
}

function FplClearanceDialog({
  stripId,
  data,
  values,
  assignedStand,
  onClose,
  onOpenMenu,
  onToggleRea,
  onToggleDeIce,
  onStandChange,
  onSidChange,
  onSelectVfr,
  onRnavChange,
  onRadisRemarksChange,
}: {
  stripId: string | null;
  data?: StripDisplayData;
  values?: StripValues;
  assignedStand: string;
  onClose: () => void;
  onOpenMenu: (
    type: MenuType,
    stripId: string,
    target?: "U2" | "arrivalRwy",
  ) => void;
  onToggleRea: (stripId: string) => void;
  onToggleDeIce: (stripId: string) => void;
  onStandChange: (stripId: string, stand: string) => void;
  onSidChange: (stripId: string, sid: string) => void;
  onSelectVfr: (stripId: string) => void;
  onRnavChange: (stripId: string, rnav: string) => void;
  onRadisRemarksChange: (stripId: string, remarks: string) => void;
}) {
  const [sidMenuOpen, setSidMenuOpen] = useState(false);

  if (!stripId || !data || !values) {
    return null;
  }

  const isEkbiDeparture =
    data.stripType === "DEPARTURE" && data.departure === "EKBI";
  const fplRunway = isEkbiDeparture && !values.U2 ? "27" : values.U2;
  const sidOptions = isEkbiDeparture
    ? Object.values(EKBI_SID_FALLBACKS[fplRunway] ?? {})
    : [];
  const sid = isEkbiDeparture
    ? values.B3 || getEkbiSid(data, fplRunway) || "NIL"
    : "NIL";

  return (
    <div
      className="clearance-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="Flight plan and clearance"
    >
      <section className="clearance-dialog">
        <fieldset className="clearance-section">
          <legend>DEPARTURE</legend>
          <div className="clearance-grid clearance-grid-top">
            <ClearanceField label="C/S" value={data.callsign} />
            <ClearanceField label="ADES" value={data.arrival} />
            <ClearanceField label="RNAV" value={values.rnav} />
            <ClearanceField
              label="SID"
              value={sid}
              interactive={isEkbiDeparture}
              onClick={() => setSidMenuOpen(true)}
            />
            <ClearanceField
              label="SSR"
              value={values.clearanceSquawk || data.assignedSquawk}
            />
            <ClearanceField label="CTOT" value="0000" />
          </div>
          <div className="clearance-grid clearance-grid-mid">
            <ClearanceField label="EOBT" value={values.U5} />
            <ClearanceField
              label="RWY"
              value={fplRunway}
              interactive
              onClick={() => onOpenMenu("RWY", stripId)}
            />
            <ClearanceField
              label="REA"
              value={values.rea}
              interactive
              onClick={() => onToggleRea(stripId)}
            />
          </div>
          <div className="clearance-grid clearance-grid-flight">
            <ClearanceField label="TYPE" value={data.aircraft} />
            <ClearanceField
              label="FL"
              value={data.altitude.replace(/^F/, "")}
            />
            <ClearanceField label="SPEED" value="" />
            <ClearanceField label="STS" value="" />
          </div>
          <ClearanceField
            label="ROUTE"
            value={data.route}
            wide
            className="clearance-route-field"
          />
          <ClearanceField label="COOPANS REMARKS" value="" wide />
          <label className="clearance-field clearance-field-wide clearance-field-editable">
            <span>RADIS REMARKS</span>
            <input
              value={values.radisRemarks}
              onChange={(event) =>
                onRadisRemarksChange(stripId, event.target.value)
              }
            />
          </label>
          <div className="clearance-grid clearance-grid-controls">
            <ClearanceField label="CLIMB GR." value="" />
            <ClearanceField
              label="HDG"
              value={values.U3}
              interactive
              onClick={() => onOpenMenu("RTE", stripId)}
            />
            <ClearanceField
              label="ALT"
              value={values.B4}
              interactive
              onClick={() => onOpenMenu("ALT", stripId)}
            />
            <ClearanceField
              label="DE-ICE"
              value={values.deIce ? "YES" : ""}
              interactive
              onClick={() => onToggleDeIce(stripId)}
            />
            <ClearanceField label="REG" value={data.registration ?? ""} />
            <label className="clearance-field clearance-field-editable">
              <span>STAND</span>
              <input
                value={assignedStand}
                onChange={(event) =>
                  onStandChange(stripId, event.target.value.toUpperCase())
                }
              />
            </label>
          </div>
        </fieldset>
        <fieldset className="clearance-section clearance-arrival-section">
          <legend>ARRIVAL</legend>
          <div className="clearance-grid clearance-grid-arrival">
            <ClearanceField label="ADEP" value={data.departure} />
            <ClearanceField label="STAR" value="" />
            <ClearanceField
              label="RWY"
              value={values.arrivalRwy}
              interactive
              onClick={() => onOpenMenu("RWY", stripId, "arrivalRwy")}
            />
            <ClearanceField label="ETA" value={values.U7} />
            <ClearanceField label="AOBT" value="" />
          </div>
        </fieldset>
        <button className="clearance-ok" onClick={onClose}>
          OK
        </button>
        {sidMenuOpen && (
          <div
            className="clearance-sid-menu"
            role="dialog"
            aria-modal="true"
            aria-label="EKBI SID selector"
          >
            <div className="clearance-sid-menu-panel">
              <div className="clearance-sid-options">
                {sidOptions.map((option) => (
                  <button
                    key={option}
                    className={option === sid ? "is-selected" : ""}
                    onClick={() => {
                      onSidChange(stripId, option);
                    }}
                  >
                    {option}
                  </button>
                ))}
              </div>
              <div className="clearance-sid-actions">
                <button type="button" onClick={() => onSelectVfr(stripId)}>
                  VFR
                </button>
                <button
                  type="button"
                  onClick={() => onRnavChange(stripId, "NON")}
                >
                  NON RNAV
                </button>
                <button
                  type="button"
                  onClick={() => onRnavChange(stripId, "1")}
                >
                  REMOVE NON RNAV
                </button>
              </div>
              <div className="clearance-sid-footer">
                <button onClick={() => onSidChange(stripId, "")}>ERASE</button>
                <button onClick={() => setSidMenuOpen(false)}>OK</button>
              </div>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

function ClearanceField({
  label,
  value,
  wide = false,
  muted = false,
  interactive = false,
  className = "",
  onClick,
}: {
  label: string;
  value: string;
  wide?: boolean;
  muted?: boolean;
  interactive?: boolean;
  className?: string;
  onClick?: () => void;
}) {
  return (
    <div
      className={`clearance-field ${wide ? "clearance-field-wide" : ""} ${interactive ? "clearance-field-editable" : "clearance-field-readonly"} ${className}`}
    >
      <span>{label}</span>
      <button disabled={!interactive} onClick={onClick}>
        {value}
      </button>
    </div>
  );
}

function SortablePrototypeStrip({
  stripId,
  stripType,
  displayData,
  stripStyle,
  displayDate,
  values,
  assignedStand,
  onOpenMenu,
  onRequestQnhUpdate,
  onToggleAta,
  onOpenFpl,
}: SortablePrototypeStripProps) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: stripId });

  const style: CSSProperties = {
    ...stripStyle,
    transform: CSS.Transform.toString(transform),
    transition,
    zIndex: isDragging ? 1 : 1,
    opacity: isDragging ? 0 : 1,
  };

  const dragHandleProps = {
    ...attributes,
    ...listeners,
  };

  return (
    <article
      className="flight-strip"
      aria-label="Prototype strip"
      style={style}
      ref={setNodeRef}
      data-strip-id={stripId}
    >
      <PrototypeStripSections
        displayDate={displayDate}
        stripId={stripId}
        stripType={stripType}
        displayData={displayData}
        values={values}
        assignedStand={assignedStand}
        onOpenMenu={onOpenMenu}
        onRequestQnhUpdate={onRequestQnhUpdate}
        onToggleAta={onToggleAta}
        onOpenFpl={onOpenFpl}
        dragHandleProps={dragHandleProps}
      />
    </article>
  );
}

function defaultStripValues(): StripValues {
  return {
    U2: "",
    B2: "",
    U3: "",
    B3: "",
    U4: "",
    B4: "",
    U5: "0000",
    B5: "",
    U6: "0000",
    B6: "",
    U7: "0000",
    B7: "",
    rea: "",
    deIce: false,
    arrivalRwy: "",
    rnav: "1",
    radisRemarks: "",
    clearanceSquawk: "",
    vfrOverride: false,
  };
}

function normalizeCallsign(value: string): string {
  return value.trim().toUpperCase();
}

function formatUtcHHMM(date: Date): string {
  return `${String(date.getUTCHours()).padStart(2, "0")}${String(date.getUTCMinutes()).padStart(2, "0")}`;
}

function normalizeIcao(value: string | undefined): string {
  return (value ?? "").trim().toUpperCase();
}

function normalizeDigits(value: string | undefined): string {
  return (value ?? "").replace(/\D/g, "");
}

function normalizeEnrouteTime(value: string | undefined): {
  hrsEnroute: string;
  minEnroute: string;
} {
  const digits = normalizeDigits(value).padStart(4, "0").slice(-4);
  return {
    hrsEnroute: digits.slice(0, 2),
    minEnroute: digits.slice(2, 4),
  };
}

function normalizeAltitude(value: string, fallback: string): string {
  const digits = value.replace(/\D/g, "");
  if (!digits) {
    return fallback;
  }

  // VATSIM often sends filed altitude in feet (e.g. 20000), while strips need flight level (F200).
  if (/^FL\d+$/i.test(value.trim())) {
    return `F${Number.parseInt(digits, 10)}`;
  }

  if (digits.length >= 4) {
    return `F${Math.floor(Number.parseInt(digits, 10) / 100)}`;
  }

  return `F${Number.parseInt(digits, 10)}`;
}

function parseRouteSegments(
  route: string,
  stripType: StripType,
): {
  firstWaypoint: string;
  firstAirwayOrSecondPoint: string;
} {
  const segments = route
    .trim()
    .toUpperCase()
    .split(/\s+/)
    .filter(
      (segment) =>
        segment.length > 0 && segment !== "DCT" && !/\d/.test(segment),
    );

  // Arrivals show the last two route points (closest to the field); departures show the first two.
  const picked =
    stripType === "ARRIVAL" ? segments.slice(-2) : segments.slice(0, 2);

  return {
    firstWaypoint: picked[0] ?? "",
    firstAirwayOrSecondPoint: picked[1] ?? "",
  };
}

const EKBI_SID_FALLBACKS: Record<string, Record<string, string>> = {
  "09": {
    ABINO: "ABINO7A",
    INTET: "INTET3A",
    RERPA: "RERPA3A",
    BAMPI: "BAMPI6A",
    MIKRO: "MIKRO6A",
    ALS: "ALS7A",
    ESIRU: "ESIRU1A",
  },
  "27": {
    ABINO: "ABINO7B",
    INTET: "INTET3B",
    RERPA: "RERPA3B",
    BAMPI: "BAMPI6B",
    MIKRO: "MIKRO6B",
    ALS: "ALS7B",
    ESIRU: "ESIRU1B",
  },
};

function getEkbiSid(data: StripDisplayData, departureRunway: string): string {
  if (data.stripType !== "DEPARTURE" || data.departure !== "EKBI") {
    return "";
  }

  const firstRouteToken = data.route
    .trim()
    .toUpperCase()
    .split(/\s+/)
    .find((segment) => segment.length > 0 && segment !== "DCT");

  if (!firstRouteToken) {
    return "";
  }

  if (/\d/.test(firstRouteToken)) {
    return firstRouteToken;
  }

  return EKBI_SID_FALLBACKS[departureRunway]?.[firstRouteToken] ?? "";
}

function addMinutesToHHMM(hhmm: string, minutesToAdd: number): string {
  const normalized = normalizeDigits(hhmm).padStart(4, "0").slice(-4);
  const hours = Number.parseInt(normalized.slice(0, 2), 10);
  const minutes = Number.parseInt(normalized.slice(2, 4), 10);
  const totalMinutes =
    (hours * 60 + minutes + minutesToAdd + 24 * 60) % (24 * 60);
  const outHours = Math.floor(totalMinutes / 60);
  const outMinutes = totalMinutes % 60;

  return `${String(outHours).padStart(2, "0")}${String(outMinutes).padStart(2, "0")}`;
}

function classifyStripType(
  departure: string,
  arrival: string,
  targetAirport: string,
): StripType | null {
  if (arrival === targetAirport) {
    return "ARRIVAL";
  }
  if (departure === targetAirport) {
    return "DEPARTURE";
  }
  return null;
}

function toDisplayData(
  seed: FlightPlanSeed,
  targetAirport: string,
): StripDisplayData | null {
  const departure = normalizeIcao(seed.departure);
  const arrival = normalizeIcao(seed.arrival);
  const stripType = classifyStripType(departure, arrival, targetAirport);
  if (!stripType) {
    return null;
  }

  const route = seed.route.trim().toUpperCase();
  const routeSegments = parseRouteSegments(route, stripType);

  return {
    callsign: seed.callsign,
    departure,
    arrival,
    aircraft: (seed.aircraft || "A320").toUpperCase(),
    wakeTurbulence: (seed.wakeTurbulence || "M").trim().toUpperCase() || "M",
    route,
    firstWaypoint: routeSegments.firstWaypoint,
    firstAirwayOrSecondPoint: routeSegments.firstAirwayOrSecondPoint,
    altitude: normalizeAltitude(seed.altitude.trim().toUpperCase(), "F0"),
    depTime: normalizeDigits(seed.depTime).slice(-4),
    hrsEnroute: normalizeDigits(seed.hrsEnroute).slice(-2),
    minEnroute: normalizeDigits(seed.minEnroute).slice(-2),
    assignedSquawk: normalizeDigits(seed.assignedSquawk).slice(-4),
    registration: seed.registration,
    flightRules: seed.vfr ? "V" : seed.flightRules,
    stripType,
    source: seed.source,
  };
}

function buildStripValuesFromDisplayData(
  data: StripDisplayData,
  departureRunway = "-",
  arrivalRunway = "-",
): StripValues {
  const plannedArrival = addMinutesToHHMM(
    data.depTime,
    Number.parseInt(data.hrsEnroute || "0", 10) * 60 +
      Number.parseInt(data.minEnroute || "0", 10) +
      15,
  );
  const sid = getEkbiSid(data, departureRunway);

  return {
    U2: data.stripType === "DEPARTURE" ? departureRunway : arrivalRunway,
    // 2/B2: starts as QNH prompt; live METAR-derived value should be written by QNH update flow.
    B2: "QNH",
    // EKBI departure SIDs are filed as the first route token or inferred from the first fix and runway.
    U3: "",
    B3: sid,
    // 4/U4: altitude shown top-left, bold, same visual scale as 5-7 values.
    U4: data.altitude,
    B4: sid ? "FL60" : "",
    // 5/U5: departure time from flightplan EOBT.
    U5: data.depTime,
    // 5/B5: reserved for system import later; intentionally blank.
    B5: "",
    // 6/U6: route duration HHMM compact code.
    U6: `${(data.hrsEnroute || "").padStart(2, "0")}${(data.minEnroute || "").padStart(2, "0")}`,
    // 6/B6: static slash by design and has no flightplan/system mapping.
    B6: "/",
    // 7/U7: planned arrival = EOBT + 15 min taxi + enroute duration.
    U7: plannedArrival,
    // 7/B7: reserved for system import later; intentionally blank.
    B7: "",
    rea: "",
    deIce: false,
    arrivalRwy: "",
    rnav: "1",
    radisRemarks: "",
    clearanceSquawk: "",
    vfrOverride: false,
  };
}

function buildStripDataFromSeeds(
  seeds: FlightPlanSeed[],
  targetAirport: string,
): Record<string, StripDisplayData> {
  const result: Record<string, StripDisplayData> = {};

  seeds.forEach((seed, index) => {
    const data = toDisplayData(seed, targetAirport);
    if (!data) {
      return;
    }
    result[
      `${data.stripType === "DEPARTURE" ? "dep" : "arr"}-strip-${index + 1}`
    ] = data;
  });

  return result;
}

function buildBaysFromStripData(
  data: Record<string, StripDisplayData>,
): BayState {
  const arrivals: string[] = [];
  const departures: string[] = [];

  Object.entries(data).forEach(([stripId, strip]) => {
    if (strip.stripType === "DEPARTURE") {
      departures.push(stripId);
    } else {
      arrivals.push(stripId);
    }
  });

  return {
    ACTIVE: [],
    RUNWAY: [],
    PASSIVE_TOP: departures,
    PASSIVE_BOTTOM: arrivals,
  };
}

function mergeBaysWithStripData(
  current: BayState,
  data: Record<string, StripDisplayData>,
): BayState {
  const presentIds = new Set(Object.keys(data));
  const knownIds = new Set<string>();

  const nextBays: BayState = {
    ACTIVE: [],
    RUNWAY: [],
    PASSIVE_TOP: [],
    PASSIVE_BOTTOM: [],
  };

  (Object.keys(nextBays) as BayId[]).forEach((bayId) => {
    current[bayId].forEach((stripId) => {
      if (presentIds.has(stripId)) {
        nextBays[bayId].push(stripId);
        knownIds.add(stripId);
      }
    });
  });

  // New strips from this poll get their default bay: departures top-right, arrivals bottom-right.
  Object.keys(data)
    .filter((stripId) => !knownIds.has(stripId))
    .forEach((stripId) => {
      if (data[stripId].stripType === "DEPARTURE") {
        nextBays.PASSIVE_TOP.push(stripId);
      } else {
        nextBays.PASSIVE_BOTTOM.push(stripId);
      }
    });

  return nextBays;
}

function buildTypesFromStripData(
  data: Record<string, StripDisplayData>,
): Record<string, StripType> {
  const result: Record<string, StripType> = {};
  Object.entries(data).forEach(([id, strip]) => {
    result[id] = strip.stripType;
  });
  return result;
}

const CARGO_CALLSIGNS = new Set([
  "AAH",
  "ABW",
  "ACP",
  "ACQ",
  "ACU",
  "ACX",
  "AEG",
  "AHC",
  "AHK",
  "AIN",
  "AKC",
  "APF",
  "ATG",
  "BBD",
  "BCS",
  "BDA",
  "BET",
  "BOX",
  "BRH",
  "CAO",
  "CBB",
  "CCC",
  "CCO",
  "CFT",
  "CGF",
  "CJT",
  "CKK",
  "CKS",
  "CLM",
  "CLU",
  "CLX",
  "CNT",
  "CRG",
  "CSB",
  "CTJ",
  "CTW",
  "CVK",
  "CWC",
  "CXM",
  "CYG",
  "DAE",
  "DCC",
  "DEC",
  "DHK",
  "DHL",
  "DHV",
  "DHX",
  "ECX",
  "EDA",
  "EFA",
  "EFE",
  "EGO",
  "EIC",
  "FAS",
  "FAX",
  "FDX",
  "FET",
  "FON",
  "FQA",
  "FRG",
  "FRH",
  "GBC",
  "GBO",
  "GCL",
  "GCO",
  "GEC",
  "GGC",
  "GJH",
  "GLC",
  "GLD",
  "GLU",
  "GNC",
  "GOL",
  "GSC",
  "GTI",
  "HEX",
  "HKC",
  "HTK",
  "HYC",
  "HYT",
  "ICL",
  "ICV",
  "IFF",
  "JAE",
  "JCH",
  "JEC",
  "JFE",
  "JUC",
  "KHA",
  "KZU",
  "LBO",
  "LCO",
  "LCR",
  "LFR",
  "LTG",
  "LUT",
  "LYB",
  "LYC",
  "MCX",
  "MDF",
  "MFD",
  "MFR",
  "MGC",
  "MGW",
  "MIX",
  "MNB",
  "MRD",
  "MRS",
  "MSA",
  "MSN",
  "MSX",
  "MTN",
  "NAB",
  "NAC",
  "NCA",
  "NCL",
  "NCR",
  "NFC",
  "NFL",
  "NKA",
  "NPT",
  "OAN",
  "OLC",
  "OWT",
  "PAC",
  "PCG",
  "PSW",
  "RAX",
  "RCG",
  "RCN",
  "REX",
  "RGD",
  "RLR",
  "RNR",
  "RTM",
  "RUN",
  "SAR",
  "SBR",
  "SCB",
  "SEV",
  "SFQ",
  "SFR",
  "SFT",
  "SHQ",
  "SLT",
  "SNC",
  "SOB",
  "SOO",
  "SUB",
  "SUK",
  "SWN",
  "SXY",
  "TAY",
  "TCG",
  "TDG",
  "TFR",
  "TMN",
  "TRG",
  "TSU",
  "TTF",
  "TTG",
  "TUM",
  "UCC",
  "UCG",
  "UPS",
  "VAS",
  "VLA",
  "VLN",
  "VRB",
  "VTG",
  "VTS",
  "WAC",
  "WAM",
  "WIS",
  "XRC",
  "UAE99",
  "QTR66",
  "THY66",
]);

function inferAircraftUseCode(aircraft: string, callsign?: string): string {
  const type = aircraft.toUpperCase();
  const normalizedCallsign = (callsign ?? "").toUpperCase();
  const callsignPrefix = normalizedCallsign.replace(/[^A-Z]/g, "").slice(0, 3);
  const callsignPattern = /^([A-Z]{3})(\d{2})$/.exec(normalizedCallsign);
  const isCargoPattern =
    Boolean(callsignPattern) &&
    ["UAE", "QTR", "THY"].includes(callsignPattern![1]);

  if (
    CARGO_CALLSIGNS.has(normalizedCallsign) ||
    CARGO_CALLSIGNS.has(callsignPrefix) ||
    isCargoPattern
  ) {
    return "C";
  }

  const isIcaoAirlineCode = /^[A-Z]{3}$/.test(callsignPrefix);
  if (isIcaoAirlineCode) {
    return "A";
  }

  if (/^(A|B|C|D|E|F|G|H|I|L|M|N|P|R|S|T|U|V|W|Y|Q)/.test(type)) {
    if (
      /^(B7|B7[0-9]|B3|B4|G|H|LJ|FA|GA|GLF|C5|C17|C1|C2|C3|C4|G[0-9]|H[0-9]|AN|IL|TU|MD|A3|A4|A3|B7)/.test(
        type,
      )
    ) {
      return "A";
    }
  }

  if (
    /^(H|S61|S76|R44|R22|MI8|MI2|EC35|UH1|AW|AS5|AS6|BK|EC|KA|NH|A139|A109|A129|A149|A158|A178)/.test(
      type,
    )
  ) {
    return "H";
  }

  if (
    /^(C5|C17|C130|C135|C141|C17|C2|C3|C5|C97|P3|P8|KC95|K35|A124|A225|A388|L101|AN12|AN22|AN32|AN70|IL76|IL86|IL96|TU95|B52|B1|B2|B3|B7|MD11|DC10)/.test(
      type,
    )
  ) {
    return "C";
  }

  return "A";
}

function inferAircraftWtc(aircraft: string): string {
  const type = aircraft.toUpperCase();
  if (
    /^(A124|A225|A388|B747|B743|B744|B748|B752|B763|B772|B773|B788|B789|L101|MD11|DC10|C17|C5|C130|A330|A340|A350|A380|A300|A310|A320|A321|A340|A350)/.test(
      type,
    )
  ) {
    return "M";
  }
  if (/^(R44|R22|A109|A139|AS50|AS55|EC35|S76|H60|H64)/.test(type)) {
    return "H";
  }
  if (
    /^(C172|PA28|P28|SR22|C152|DA40|TBM|BE58|PA34|GLEX|CJ|CL|GLF|FA7|FA8|PC12|DO228|DHC6|AT72|Q400|DH8)/.test(
      type,
    )
  ) {
    return "L";
  }
  return "M";
}

function findStandForStrip(
  strip: StripDisplayData,
  standAssignments: StandAssignment[],
  aircraftPosition?: PilotPosition,
  lastKnownStand?: string,
  selectedAirport: string = DEFAULT_AIRPORT,
  occupiedStandNumbers: string[] = [],
): string {
  const airport =
    selectedAirport || strip.arrival || strip.departure || DEFAULT_AIRPORT;
  const aircraftPositionValid =
    typeof aircraftPosition?.latitude === "number" &&
    typeof aircraftPosition?.longitude === "number";

  if (strip.stripType === "DEPARTURE") {
    if (!aircraftPositionValid) {
      return "";
    }

    return (
      findStandAtPosition(
        standAssignments,
        airport,
        aircraftPosition.latitude!,
        aircraftPosition.longitude!,
      )?.stand ?? ""
    );
  }

  const aircraftUse = inferAircraftUseCode(strip.aircraft, strip.callsign);
  const aircraftWtc = inferAircraftWtc(strip.aircraft);
  const airportIsSchengen = isSchengenAirport(airport);

  if (!aircraftPositionValid) {
    if (lastKnownStand) {
      return lastKnownStand;
    }
    return "";
  }

  if (import.meta.env.DEV) {
    console.log("stand-match-debug", {
      callsign: strip.callsign,
      airport,
      aircraftPosition: {
        latitude: aircraftPosition?.latitude,
        longitude: aircraftPosition?.longitude,
      },
      standCount: standAssignments.filter((stand) => stand.airport === airport)
        .length,
    });
  }

  const candidates = standAssignments.filter((stand) => {
    if (
      stand.airport !== airport ||
      occupiedStandNumbers.includes(stand.stand)
    ) {
      return false;
    }

    if (stand.use && stand.use.length > 0) {
      const allowed = stand.use.toUpperCase();
      if (!allowed.includes(aircraftUse)) {
        return false;
      }
    }

    if (typeof stand.schengen === "boolean") {
      if (stand.schengen && !airportIsSchengen) {
        return false;
      }
      if (stand.nonSchengen && airportIsSchengen) {
        return false;
      }
    }

    if (
      stand.coord1 &&
      stand.coord2 &&
      isStandBlockedByRadius(
        stand,
        aircraftPosition.latitude!,
        aircraftPosition.longitude!,
      )
    ) {
      return false;
    }

    if (stand.wtc && stand.wtc.length > 0 && !stand.wtc.includes(aircraftWtc)) {
      return false;
    }

    return true;
  });

  const bestCandidate = candidates.sort(
    (left, right) => (right.priority ?? 0) - (left.priority ?? 0),
  )[0];

  if (bestCandidate) {
    return bestCandidate.stand;
  }

  if (lastKnownStand) {
    return lastKnownStand;
  }

  return "";
}

async function fetchFlightPlansForAirport(
  targetAirport: string,
  restrictToAirportRadius = true,
): Promise<FlightPlanSeed[]> {
  const response = await fetch(VATSIM_DATA_URL);
  if (!response.ok) {
    throw new Error(`VATSIM feed request failed: ${response.status}`);
  }

  const payload = (await response.json()) as VatsimDataResponse;
  const pilots = payload.pilots ?? [];

  const flights = pilots.reduce<FlightPlanSeed[]>((acc, pilot) => {
    if (!pilot.callsign || !pilot.flight_plan) {
      return acc;
    }

    if (
      restrictToAirportRadius &&
      !isWithinAirportRadius(targetAirport, pilot.latitude, pilot.longitude)
    ) {
      return acc;
    }

    const departure = normalizeIcao(pilot.flight_plan?.departure);
    const arrival = normalizeIcao(pilot.flight_plan?.arrival);
    const stripType = classifyStripType(departure, arrival, targetAirport);
    if (!stripType) {
      return acc;
    }

    acc.push({
      callsign: pilot.callsign.trim().toUpperCase(),
      departure,
      arrival,
      aircraft: normalizeIcao(pilot.flight_plan?.aircraft_short) || "A320",
      wakeTurbulence: normalizeIcao(pilot.flight_plan?.wake_turbulence) || "M",
      route: (pilot.flight_plan?.route ?? "").trim().toUpperCase(),
      altitude: (pilot.flight_plan?.altitude ?? "").trim().toUpperCase(),
      depTime: normalizeDigits(pilot.flight_plan?.deptime),
      ...normalizeEnrouteTime(pilot.flight_plan?.enroute_time),
      assignedSquawk: normalizeDigits(pilot.flight_plan?.assigned_transponder),
      registration: "",
      flightRules: pilot.flight_plan?.flight_rules === "V" ? "V" : "I",
      source: "vatsim" as const,
      latitude: typeof pilot.latitude === "number" ? pilot.latitude : undefined,
      longitude:
        typeof pilot.longitude === "number" ? pilot.longitude : undefined,
      liveAltitudeFt:
        typeof pilot.altitude === "number" ? pilot.altitude : null,
    });

    return acc;
  }, []);

  return flights.slice(0, 10);
}

export default function App() {
  const leftColumnRef = useRef<HTMLElement | null>(null);
  const dragStateRef = useRef({
    active: false,
    startY: 0,
    startHeight: 0,
  });
  // Callsign -> ATD (HHMM) the moment altitude first exceeded 500ft; frozen once captured.
  const atdTimestampsRef = useRef<Record<string, string>>({});
  // Callsigns that have ever been shown as a strip this session; used to gate new high-altitude departures.
  const knownCallsignsRef = useRef<Set<string>>(new Set());

  const [stripData, setStripData] = useState<Record<string, StripDisplayData>>(
    {},
  );

  const [runwayHeight, setRunwayHeight] = useState(210);
  const [stripHeightPx, setStripHeightPx] = useState(
    () => window.screen.height * 0.08,
  );
  const [activeDragId, setActiveDragId] = useState<string | null>(null);
  const [menuState, setMenuState] = useState<MenuState | null>(null);
  const [fplStripId, setFplStripId] = useState<string | null>(null);
  const [menuValue, setMenuValue] = useState("");
  const [bays, setBays] = useState<BayState>(() =>
    buildBaysFromStripData(stripData),
  );
  const [stripValues, setStripValues] = useState<Record<string, StripValues>>(
    () => {
      const initial: Record<string, StripValues> = {};
      Object.entries(stripData).forEach(([id, data]) => {
        initial[id] = buildStripValuesFromDisplayData(data);
      });
      return initial;
    },
  );
  const [hiddenCallsigns, setHiddenCallsigns] = useState<Set<string>>(
    () => new Set(),
  );
  const [findDialogOpen, setFindDialogOpen] = useState(false);
  const [plannedStripData, setPlannedStripData] = useState<
    Record<string, StripDisplayData>
  >({});
  const [newFplDialogOpen, setNewFplDialogOpen] = useState(false);
  const [newFplInitialCallsign, setNewFplInitialCallsign] = useState("");
  const [newFplInitialStripId, setNewFplInitialStripId] = useState<
    string | null
  >(null);
  const [departureRunway, setDepartureRunway] = useState("-");
  const [arrivalRunway, setArrivalRunway] = useState("-");
  const [selectedAirport, setSelectedAirport] = useState(getStoredAirport);
  const [standAssignments, setStandAssignments] = useState<StandAssignment[]>(
    [],
  );
  const [stripStandAssignments, setStripStandAssignments] = useState<
    Record<string, string>
  >({});
  const lastKnownStandRef = useRef<Record<string, string>>({});
  const manualStandOverridesRef = useRef<Record<string, string>>({});
  const lockedClearanceFieldsRef = useRef<
    Record<string, Set<"U2" | "U3" | "B3" | "B4" | "rnav" | "radisRemarks">>
  >({});
  const pilotPositionRef = useRef<Record<string, PilotPosition>>({});

  function lockClearanceField(
    stripId: string,
    field: "U2" | "U3" | "B3" | "B4" | "rnav" | "radisRemarks",
  ) {
    const fields = lockedClearanceFieldsRef.current[stripId] ?? new Set();
    fields.add(field);
    lockedClearanceFieldsRef.current[stripId] = fields;
  }

  useEffect(() => {
    try {
      window.localStorage.setItem(
        SELECTED_AIRPORT_STORAGE_KEY,
        selectedAirport,
      );
    } catch {
      // The application remains usable when browser storage is unavailable.
    }
  }, [selectedAirport]);

  useEffect(() => {
    setStripValues((current) => {
      const next = { ...current };

      Object.entries(stripData).forEach(([stripId, data]) => {
        const runway =
          data.stripType === "DEPARTURE" ? departureRunway : arrivalRunway;
        const values = next[stripId] ?? defaultStripValues();
        const sid = values.vfrOverride
          ? "VFR"
          : getEkbiSid(data, departureRunway);
        const enteredLevel = sid && !values.B4 ? "FL60" : values.B4;
        const lockedFields = lockedClearanceFieldsRef.current[stripId];
        const nextRunway = lockedFields?.has("U2") ? values.U2 : runway;
        const nextSid = lockedFields?.has("B3") ? values.B3 : sid;
        const nextLevel = lockedFields?.has("B4") ? values.B4 : enteredLevel;
        if (
          values.U2 !== nextRunway ||
          values.B3 !== nextSid ||
          values.B4 !== nextLevel
        ) {
          next[stripId] = {
            ...values,
            U2: nextRunway,
            B3: nextSid,
            B4: nextLevel,
          };
        }
      });

      return next;
    });
  }, [arrivalRunway, departureRunway, stripData]);

  useEffect(() => {
    async function loadStandAssignments() {
      try {
        const response = await fetch("/standassignment.txt");
        if (!response.ok) {
          return;
        }

        const text = await response.text();
        setStandAssignments(parseStandAssignmentFile(text));
      } catch {
        // Keep the app working even if the stand file is not available.
      }
    }

    loadStandAssignments();
  }, []);

  const visibleStripData = Object.fromEntries(
    Object.entries(stripData).filter(
      ([, data]) => !hiddenCallsigns.has(data.callsign.toUpperCase()),
    ),
  ) as Record<string, StripDisplayData>;
  const visibleStripTypes = buildTypesFromStripData(visibleStripData);

  useEffect(() => {
    const visible = Object.fromEntries(
      Object.entries(stripData).filter(
        ([, data]) => !hiddenCallsigns.has(data.callsign.toUpperCase()),
      ),
    ) as Record<string, StripDisplayData>;
    setBays((current) => mergeBaysWithStripData(current, visible));
  }, [hiddenCallsigns, stripData]);

  useEffect(() => {
    let active = true;
    atdTimestampsRef.current = {};
    knownCallsignsRef.current = new Set();

    async function loadFlightPlans() {
      try {
        const vatsimSeeds = await fetchFlightPlansForAirport(selectedAirport);

        if (!active) {
          return;
        }

        pilotPositionRef.current = Object.fromEntries(
          vatsimSeeds
            .filter(
              (seed) =>
                typeof seed.latitude === "number" &&
                typeof seed.longitude === "number",
            )
            .map((seed) => {
              const key = normalizeCallsign(seed.callsign);
              return [
                key,
                {
                  latitude: seed.latitude,
                  longitude: seed.longitude,
                },
              ];
            }),
        );

        // New departures already above 14000ft are held back until they're first seen below that; strips already shown stay.
        const admittedSeeds = vatsimSeeds.filter((seed) => {
          const callsign = normalizeCallsign(seed.callsign);
          if (knownCallsignsRef.current.has(callsign)) {
            return true;
          }

          const stripType = classifyStripType(
            seed.departure,
            seed.arrival,
            selectedAirport,
          );
          if (
            stripType === "DEPARTURE" &&
            typeof seed.liveAltitudeFt === "number" &&
            seed.liveAltitudeFt > 14000
          ) {
            return false;
          }

          return true;
        });

        admittedSeeds.forEach((seed) => {
          knownCallsignsRef.current.add(normalizeCallsign(seed.callsign));
        });

        const nextData = buildStripDataFromSeeds(
          admittedSeeds,
          selectedAirport,
        );

        const nowHHMM = formatUtcHHMM(new Date());
        const newAtd: Record<string, string> = {};
        admittedSeeds.forEach((seed) => {
          if (
            typeof seed.liveAltitudeFt !== "number" ||
            seed.liveAltitudeFt <= 500
          ) {
            return;
          }
          const callsign = normalizeCallsign(seed.callsign);
          if (atdTimestampsRef.current[callsign]) {
            return;
          }
          newAtd[callsign] = nowHHMM;
        });

        if (Object.keys(newAtd).length > 0) {
          atdTimestampsRef.current = { ...atdTimestampsRef.current, ...newAtd };
        }

        setStripData((current) => {
          const persistentStrips = Object.fromEntries(
            Object.entries(current).filter(
              ([, strip]) =>
                strip.source === "manual" || strip.source === "planned",
            ),
          );
          const persistentCallsigns = new Set(
            Object.values(persistentStrips).map((strip) =>
              normalizeCallsign(strip.callsign),
            ),
          );
          const liveStrips = Object.fromEntries(
            Object.entries(nextData).filter(
              ([, strip]) =>
                !persistentCallsigns.has(normalizeCallsign(strip.callsign)),
            ),
          );
          return { ...liveStrips, ...persistentStrips };
        });
        setStripValues((current) => {
          const next = { ...current };
          Object.entries(nextData).forEach(([stripId, data]) => {
            const callsign = normalizeCallsign(data.callsign);
            const knownTimestamp = atdTimestampsRef.current[callsign];

            if (!next[stripId]) {
              const built = buildStripValuesFromDisplayData(data);
              if (knownTimestamp) {
                built.B5 = knownTimestamp;
              }
              next[stripId] = built;
            } else if (newAtd[callsign] && !next[stripId].B5) {
              next[stripId] = { ...next[stripId], B5: newAtd[callsign] };
            }
          });
          return next;
        });

        setStripStandAssignments((current) => {
          const next: Record<string, string> = { ...current };
          const occupiedStandNumbers = new Set<string>();

          Object.entries(current).forEach(([stripId, assignedStand]) => {
            const currentStrip = stripData[stripId];
            if (
              assignedStand &&
              (!nextData[stripId] || currentStrip?.stripType === "DEPARTURE")
            ) {
              occupiedStandNumbers.add(assignedStand);
            }
          });

          Object.entries(nextData)
            .filter(([, data]) => data.stripType === "DEPARTURE")
            .forEach(([stripId, data]) => {
              const manualStand = manualStandOverridesRef.current[stripId];
              const pilotPosition =
                pilotPositionRef.current[normalizeCallsign(data.callsign)];
              const assignedStand =
                manualStand ??
                findStandForStrip(
                  data,
                  standAssignments,
                  pilotPosition,
                  lastKnownStandRef.current[stripId],
                  selectedAirport,
                );
              next[stripId] = assignedStand;
              if (assignedStand) {
                occupiedStandNumbers.add(assignedStand);
                lastKnownStandRef.current[stripId] = assignedStand;
              }
            });

          Object.entries(nextData)
            .filter(([, data]) => data.stripType === "ARRIVAL")
            .forEach(([stripId, data]) => {
              const manualStand = manualStandOverridesRef.current[stripId];
              if (manualStand !== undefined) {
                next[stripId] = manualStand;
                return;
              }

              const pilotPosition =
                pilotPositionRef.current[normalizeCallsign(data.callsign)];
              const assignedStand = findStandForStrip(
                data,
                standAssignments,
                pilotPosition,
                lastKnownStandRef.current[stripId],
                selectedAirport,
                Array.from(occupiedStandNumbers),
              );
              next[stripId] = assignedStand;
              if (assignedStand) {
                occupiedStandNumbers.add(assignedStand);
                lastKnownStandRef.current[stripId] = assignedStand;
              }
            });
          return next;
        });
      } catch {
        // Keep current strips when VATSIM is unavailable.
      }
    }

    setStripData({});
    loadFlightPlans();

    const intervalId = window.setInterval(loadFlightPlans, 15_000);
    return () => {
      active = false;
      window.clearInterval(intervalId);
    };
  }, [selectedAirport]);

  useEffect(() => {
    function onMouseMove(event: MouseEvent) {
      if (!dragStateRef.current.active || !leftColumnRef.current) {
        return;
      }

      const totalHeight = leftColumnRef.current.clientHeight;
      const minRunwayHeight = 100;
      const minActiveHeight = 150;
      const maxRunwayHeight = Math.max(
        minRunwayHeight,
        totalHeight - minActiveHeight,
      );

      const delta = dragStateRef.current.startY - event.clientY;
      const nextHeight = dragStateRef.current.startHeight + delta;
      const clampedHeight = Math.min(
        maxRunwayHeight,
        Math.max(minRunwayHeight, nextHeight),
      );

      setRunwayHeight(clampedHeight);
    }

    function onMouseUp() {
      if (!dragStateRef.current.active) {
        return;
      }

      dragStateRef.current.active = false;
      document.body.classList.remove("is-resizing-runway");
    }

    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);

    return () => {
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
      document.body.classList.remove("is-resizing-runway");
    };
  }, []);

  useEffect(() => {
    function syncStripHeightToScreen() {
      setStripHeightPx(window.screen.height * 0.08);
    }

    window.addEventListener("resize", syncStripHeightToScreen);
    return () => {
      window.removeEventListener("resize", syncStripHeightToScreen);
    };
  }, []);

  function startRunwayResize(event: React.MouseEvent<HTMLDivElement>) {
    dragStateRef.current.active = true;
    dragStateRef.current.startY = event.clientY;
    dragStateRef.current.startHeight = runwayHeight;
    document.body.classList.add("is-resizing-runway");
  }

  const today = new Date();
  const displayDate = `${String(today.getDate()).padStart(2, "0")}/${String(
    today.getMonth() + 1,
  ).padStart(2, "0")}/${today.getFullYear()}`;

  function requestQnhUpdate(stripId: string) {
    // B2/QNH: for now we always read METAR for EKYT; later this should use logged-in airport context.
    fetch(`https://metar.vatsim.net/${selectedAirport}`)
      .then((response) => response.text())
      .then((metar) => {
        const match = metar.match(/\bQ(\d{4})\b/);
        const qnhValue = match ? `Q${match[1]}` : "Q1013";

        setStripValues((latest) => ({
          ...latest,
          [stripId]: {
            ...(latest[stripId] ?? defaultStripValues()),
            B2: qnhValue,
          },
        }));
      })
      .catch(() => {
        // Fake API fallback for local testing when live METAR is unavailable or blocked.
        setStripValues((latest) => ({
          ...latest,
          [stripId]: {
            ...(latest[stripId] ?? defaultStripValues()),
            B2: "Q1013",
          },
        }));
      });
  }

  function toggleAta(stripId: string) {
    setStripValues((latest) => {
      const current = latest[stripId] ?? defaultStripValues();
      return {
        ...latest,
        [stripId]: {
          ...current,
          B7: current.B7 ? "" : formatUtcHHMM(new Date()),
        },
      };
    });
  }

  function toggleRea(stripId: string) {
    setStripValues((current) => {
      const values = current[stripId] ?? defaultStripValues();
      return {
        ...current,
        [stripId]: {
          ...values,
          rea: values.rea ? "" : formatUtcHHMM(new Date()),
        },
      };
    });
  }

  function toggleDeIce(stripId: string) {
    setStripValues((current) => {
      const values = current[stripId] ?? defaultStripValues();
      return {
        ...current,
        [stripId]: { ...values, deIce: !values.deIce },
      };
    });
  }

  function updateAssignedStand(stripId: string, stand: string) {
    if (!stand) {
      delete manualStandOverridesRef.current[stripId];
      const strip = stripData[stripId];
      const pilotPosition = strip
        ? pilotPositionRef.current[normalizeCallsign(strip.callsign)]
        : undefined;
      const sensedStand = strip
        ? findStandForStrip(
            strip,
            standAssignments,
            pilotPosition,
            lastKnownStandRef.current[stripId],
            selectedAirport,
          )
        : "";
      setStripStandAssignments((current) => ({
        ...current,
        [stripId]: sensedStand,
      }));
      return;
    }

    manualStandOverridesRef.current[stripId] = stand;
    setStripStandAssignments((current) => ({ ...current, [stripId]: stand }));
  }

  function updateSid(stripId: string, sid: string) {
    lockClearanceField(stripId, "B3");
    setStripValues((current) => ({
      ...current,
      [stripId]: {
        ...(current[stripId] ?? defaultStripValues()),
        B3: sid,
      },
    }));
  }

  function selectVfr(stripId: string) {
    lockClearanceField(stripId, "B3");
    setStripValues((current) => ({
      ...current,
      [stripId]: {
        ...(current[stripId] ?? defaultStripValues()),
        B3: "VFR",
        clearanceSquawk: "7000",
        vfrOverride: true,
      },
    }));
  }

  function updateRnav(stripId: string, rnav: string) {
    lockClearanceField(stripId, "rnav");
    setStripValues((current) => ({
      ...current,
      [stripId]: {
        ...(current[stripId] ?? defaultStripValues()),
        rnav,
      },
    }));
  }

  function updateRadisRemarks(stripId: string, remarks: string) {
    lockClearanceField(stripId, "radisRemarks");
    setStripValues((current) => ({
      ...current,
      [stripId]: {
        ...(current[stripId] ?? defaultStripValues()),
        radisRemarks: remarks,
      },
    }));
  }

  async function handleOpenFindDialog() {
    setFindDialogOpen(true);
    try {
      const plannedSeeds = await fetchFlightPlansForAirport(
        selectedAirport,
        false,
      );
      setPlannedStripData(
        buildStripDataFromSeeds(plannedSeeds, selectedAirport),
      );
    } catch {
      setPlannedStripData({});
    }
  }

  function handleSelectPlannedStrip(plannedStripId: string) {
    const plannedStrip = plannedStripData[plannedStripId];
    if (!plannedStrip) {
      return;
    }

    const callsign = normalizeCallsign(plannedStrip.callsign);
    setHiddenCallsigns((current) => {
      if (!current.has(callsign)) {
        return current;
      }

      const next = new Set(current);
      next.delete(callsign);
      return next;
    });

    const existingStripId = Object.entries(stripData).find(
      ([, strip]) =>
        normalizeCallsign(strip.callsign) ===
        callsign,
    )?.[0];
    const stripId =
      existingStripId ?? `planned-${plannedStrip.callsign}-${Date.now()}`;

    setStripData((current) => ({
      ...current,
      [stripId]: { ...plannedStrip, source: "planned" },
    }));
    if (!existingStripId) {
      setStripValues((current) => ({
        ...current,
        [stripId]: buildStripValuesFromDisplayData(plannedStrip),
      }));
    }

    setBays((current) => {
      const removeStrip = (stripIds: string[]) =>
        stripIds.filter((id) => id !== stripId);
      const targetBay =
        plannedStrip.stripType === "DEPARTURE"
          ? "PASSIVE_TOP"
          : "PASSIVE_BOTTOM";
      return {
        ...current,
        ACTIVE: removeStrip(current.ACTIVE),
        RUNWAY: removeStrip(current.RUNWAY),
        PASSIVE_TOP:
          targetBay === "PASSIVE_TOP"
            ? [...removeStrip(current.PASSIVE_TOP), stripId]
            : removeStrip(current.PASSIVE_TOP),
        PASSIVE_BOTTOM:
          targetBay === "PASSIVE_BOTTOM"
            ? [...removeStrip(current.PASSIVE_BOTTOM), stripId]
            : removeStrip(current.PASSIVE_BOTTOM),
      };
    });
    setFindDialogOpen(false);
  }

  function handleOpenFpl(stripId: string) {
    setFplStripId(stripId);
  }

  function handleOpenNewDialog(initialCallsign = "") {
    setNewFplInitialCallsign(initialCallsign);
    setNewFplInitialStripId(null);
    setNewFplDialogOpen(true);
  }

  function handleOpenExistingStrip(callsign: string) {
    const normalized = normalizeCallsign(callsign);
    setHiddenCallsigns((current) => {
      if (!current.has(normalized)) {
        return current;
      }

      const next = new Set(current);
      next.delete(normalized);
      return next;
    });

    const existingEntry = Object.entries(stripData).find(
      ([, strip]) => normalizeCallsign(strip.callsign) === normalized,
    );
    if (existingEntry) {
      const [stripId] = existingEntry;
      setNewFplInitialCallsign(normalized);
      setNewFplInitialStripId(stripId);
      setNewFplDialogOpen(true);
    }
  }

  function handleRequestNewFromFind(callsign: string) {
    handleOpenNewDialog(callsign);
  }

  function handleCreateStrip(seed: FlightPlanSeed) {
    const displayData = toDisplayData(seed, selectedAirport);
    if (!displayData) {
      return;
    }

    const stripId = `manual-${displayData.callsign}-${Date.now()}`;
    setStripData((current) => ({
      ...current,
      [stripId]: displayData,
    }));
    setStripValues((current) => ({
      ...current,
      [stripId]: {
        ...buildStripValuesFromDisplayData(displayData),
        U2: seed.assignedRunway || departureRunway,
        B3: seed.sid || getEkbiSid(displayData, departureRunway),
        rnav: seed.rnav ?? "1",
        radisRemarks: seed.radisRemarks ?? "",
        clearanceSquawk: seed.vfr ? "7000" : "",
        vfrOverride: seed.vfr ?? false,
      },
    }));
    setHiddenCallsigns((current) => {
      if (!current.has(displayData.callsign.toUpperCase())) {
        return current;
      }

      const next = new Set(current);
      next.delete(displayData.callsign.toUpperCase());
      return next;
    });
  }

  function openMenu(
    type: MenuType,
    stripId: string,
    field?: NumField,
    target?: "U2" | "arrivalRwy",
  ) {
    setMenuState({ type, stripId, field, target });

    const values = stripValues[stripId] ?? defaultStripValues();
    if (type === "NUM" && field) {
      if (field === "B6" && values[field] === "/") {
        setMenuValue("");
      } else {
        setMenuValue(values[field]);
      }
      return;
    }
    if (type === "ALT") {
      setMenuValue(values.B4);
      return;
    }
    if (type === "RTE") {
      setMenuValue("");
      return;
    }
    setMenuValue("");
  }

  function closeMenu() {
    setMenuState(null);
    setMenuValue("");
  }

  function submitMenuValue(nextValue: string) {
    if (!menuState) {
      return;
    }

    const cleaned = nextValue.trim().toUpperCase();

    if (menuState.type === "FPL") {
      closeMenu();
      return;
    }

    setStripValues((current) => {
      const strip = current[menuState.stripId] ?? defaultStripValues();

      if (menuState.type === "RTE") {
        if (/^\d{3}$/.test(cleaned)) {
          lockClearanceField(menuState.stripId, "U3");
          lockClearanceField(menuState.stripId, "B3");
          return {
            ...current,
            [menuState.stripId]: {
              ...strip,
              B3: "",
              U3: cleaned,
            },
          };
        }

        if (/^[A-Z]{3}$/.test(cleaned) || /^[A-Z]{5}$/.test(cleaned)) {
          lockClearanceField(menuState.stripId, "U3");
          lockClearanceField(menuState.stripId, "B3");
          return {
            ...current,
            [menuState.stripId]: {
              ...strip,
              U3: "",
              B3: cleaned,
            },
          };
        }

        return current;
      }

      if (menuState.type === "ALT") {
        if (!cleaned) {
          return current;
        }

        lockClearanceField(menuState.stripId, "B4");

        return {
          ...current,
          [menuState.stripId]: {
            ...strip,
            B4: cleaned,
          },
        };
      }

      if (menuState.type === "RWY") {
        if (!cleaned) {
          return current;
        }

        if ((menuState.target ?? "U2") === "U2") {
          lockClearanceField(menuState.stripId, "U2");
        }

        return {
          ...current,
          [menuState.stripId]: {
            ...strip,
            [menuState.target ?? "U2"]: cleaned,
          },
        };
      }

      if (menuState.type === "NUM" && menuState.field) {
        const normalized = cleaned.replace(/[^0-9./]/g, "");
        if (!normalized) {
          return current;
        }

        return {
          ...current,
          [menuState.stripId]: {
            ...strip,
            [menuState.field]: normalized,
          },
        };
      }

      return current;
    });

    closeMenu();
  }

  function confirmMenu() {
    submitMenuValue(menuValue);
  }

  function handleStripDragStart(event: DragStartEvent) {
    setActiveDragId(String(event.active.id));
  }

  function findContainer(itemId: string, state: BayState): BayId | null {
    if (
      itemId === "ACTIVE" ||
      itemId === "RUNWAY" ||
      itemId === "PASSIVE_TOP" ||
      itemId === "PASSIVE_BOTTOM"
    ) {
      return itemId;
    }

    if (state.ACTIVE.includes(itemId)) return "ACTIVE";
    if (state.RUNWAY.includes(itemId)) return "RUNWAY";
    if (state.PASSIVE_TOP.includes(itemId)) return "PASSIVE_TOP";
    if (state.PASSIVE_BOTTOM.includes(itemId)) return "PASSIVE_BOTTOM";
    return null;
  }

  function handleStripDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over) {
      return;
    }

    const activeId = String(active.id);
    const overId = String(over.id);

    if (overId === DELETE_BAY_ID) {
      const strip = stripData[activeId];
      if (strip) {
        const callsign = normalizeCallsign(strip.callsign);
        setHiddenCallsigns((current) => {
          if (current.has(callsign)) {
            return current;
          }

          const next = new Set(current);
          next.add(callsign);
          return next;
        });

        if (strip.source === "manual" || strip.source === "planned") {
          setStripData((current) => {
            const next = { ...current };
            delete next[activeId];
            return next;
          });
          setStripValues((current) => {
            const next = { ...current };
            delete next[activeId];
            return next;
          });
          setStripStandAssignments((current) => {
            const next = { ...current };
            delete next[activeId];
            return next;
          });
          setBays((current) => ({
            ACTIVE: current.ACTIVE.filter((id) => id !== activeId),
            RUNWAY: current.RUNWAY.filter((id) => id !== activeId),
            PASSIVE_TOP: current.PASSIVE_TOP.filter((id) => id !== activeId),
            PASSIVE_BOTTOM: current.PASSIVE_BOTTOM.filter(
              (id) => id !== activeId,
            ),
          }));
          setActiveDragId(null);
          return;
        }
      }
      setActiveDragId(null);
      return;
    }

    setBays((current) => {
      const sourceBay = findContainer(activeId, current);
      const targetBay = findContainer(overId, current);

      if (!sourceBay || !targetBay) {
        return current;
      }

      if (sourceBay === targetBay) {
        const items = current[sourceBay];
        const oldIndex = items.indexOf(activeId);
        const overIndex = items.indexOf(overId);

        if (oldIndex === -1) {
          return current;
        }

        const nextItems =
          overIndex === -1 ? items : arrayMove(items, oldIndex, overIndex);

        return {
          ...current,
          [sourceBay]: nextItems,
        };
      }

      const sourceItems = [...current[sourceBay]];
      const targetItems = [...current[targetBay]];
      const sourceIndex = sourceItems.indexOf(activeId);

      if (sourceIndex === -1) {
        return current;
      }

      sourceItems.splice(sourceIndex, 1);

      const targetIndex = targetItems.indexOf(overId);
      if (targetIndex === -1) {
        targetItems.push(activeId);
      } else {
        targetItems.splice(targetIndex, 0, activeId);
      }

      return {
        ...current,
        [sourceBay]: sourceItems,
        [targetBay]: targetItems,
      };
    });

    setActiveDragId(null);
  }

  function handleStripDragCancel() {
    setActiveDragId(null);
  }

  const collisionDetection = ((args) => {
    const pointerCollisions = pointerWithin(args);
    if (pointerCollisions.length > 0) {
      return pointerCollisions;
    }

    const bayCollisions = closestCenter({
      ...args,
      droppableContainers: args.droppableContainers.filter((container) => {
        const id = String(container.id);
        return (
          id === "ACTIVE" ||
          id === "RUNWAY" ||
          id === "PASSIVE_TOP" ||
          id === "PASSIVE_BOTTOM" ||
          id === DELETE_BAY_ID
        );
      }),
    });

    if (bayCollisions.length > 0) {
      return bayCollisions;
    }

    return closestCenter(args);
  }) satisfies CollisionDetection;

  const STRIP_DRAG_PRESS_DELAY_MS = 20;

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        delay: STRIP_DRAG_PRESS_DELAY_MS,
        tolerance: 8,
      },
    }),
  );

  const stripStyle = {
    height: `${stripHeightPx}px`,
    width: `${stripHeightPx * 9}px`,
    "--strip-h": `${stripHeightPx}px`,
  } as CSSProperties;

  return (
    <div className="layout-root">
      <DndContext
        sensors={sensors}
        collisionDetection={collisionDetection}
        onDragStart={handleStripDragStart}
        onDragCancel={handleStripDragCancel}
        onDragEnd={handleStripDragEnd}
      >
        <main className="bay-canvas">
          <section className="bay-column left-column" ref={leftColumnRef}>
            <div className="bay-top-strip" aria-hidden />

            <article className="bay active-bay">
              <header className="bay-title">ACTIVE</header>
              <SortableStripList
                bayId="ACTIVE"
                stripIds={bays.ACTIVE}
                stripStyle={stripStyle}
                displayDate={displayDate}
                stripValues={stripValues}
                stripTypes={visibleStripTypes}
                stripData={stripData}
                stripStandAssignments={stripStandAssignments}
                onOpenMenu={openMenu}
                onRequestQnhUpdate={requestQnhUpdate}
                onToggleAta={toggleAta}
                onOpenFpl={handleOpenFpl}
              />
            </article>

            <div
              className="runway-resize-handle"
              role="separator"
              aria-label="Resize runway bay"
              aria-orientation="horizontal"
              onMouseDown={startRunwayResize}
            />

            <article
              className="bay runway-bay"
              style={{ height: `${runwayHeight}px` }}
            >
              <header className="bay-title">RUNWAY</header>
              <SortableStripList
                bayId="RUNWAY"
                stripIds={bays.RUNWAY}
                stripStyle={stripStyle}
                displayDate={displayDate}
                stripValues={stripValues}
                stripTypes={visibleStripTypes}
                stripData={stripData}
                stripStandAssignments={stripStandAssignments}
                onOpenMenu={openMenu}
                onRequestQnhUpdate={requestQnhUpdate}
                onToggleAta={toggleAta}
                onOpenFpl={handleOpenFpl}
              />
            </article>
          </section>

          <div className="column-divider" aria-hidden />

          <section className="bay-column right-column">
            <div className="bay-top-strip" aria-hidden />

            <article className="bay passive-bay">
              <header className="bay-title">PASSIVE</header>
              <div className="passive-sections">
                <div className="passive-half passive-half-top">
                  <SortableStripList
                    bayId="PASSIVE_TOP"
                    stripIds={bays.PASSIVE_TOP}
                    stripStyle={stripStyle}
                    displayDate={displayDate}
                    stripValues={stripValues}
                    stripTypes={visibleStripTypes}
                    stripData={stripData}
                    stripStandAssignments={stripStandAssignments}
                    onOpenMenu={openMenu}
                    onRequestQnhUpdate={requestQnhUpdate}
                    onToggleAta={toggleAta}
                    onOpenFpl={handleOpenFpl}
                  />
                </div>

                <div className="passive-half passive-half-bottom">
                  <SortableStripList
                    bayId="PASSIVE_BOTTOM"
                    stripIds={bays.PASSIVE_BOTTOM}
                    stripStyle={stripStyle}
                    displayDate={displayDate}
                    stripValues={stripValues}
                    stripTypes={visibleStripTypes}
                    stripData={stripData}
                    stripStandAssignments={stripStandAssignments}
                    onOpenMenu={openMenu}
                    onRequestQnhUpdate={requestQnhUpdate}
                    onToggleAta={toggleAta}
                    onOpenFpl={handleOpenFpl}
                    reverse
                  />
                </div>
              </div>
            </article>
          </section>
        </main>

        <DragOverlay>
          {activeDragId ? (
            <article
              className="flight-strip flight-strip-overlay"
              style={stripStyle}
            >
              <PrototypeStripSections
                displayDate={displayDate}
                stripId={activeDragId}
                stripType={visibleStripTypes[activeDragId] ?? "ARRIVAL"}
                displayData={
                  stripData[activeDragId] ?? {
                    callsign: "N/A",
                    departure: selectedAirport,
                    arrival: "EKCH",
                    aircraft: "A320",
                    wakeTurbulence: "M",
                    route: "",
                    firstWaypoint: "",
                    firstAirwayOrSecondPoint: "",
                    altitude: "",
                    depTime: "",
                    hrsEnroute: "",
                    minEnroute: "",
                    assignedSquawk: "",
                    stripType: visibleStripTypes[activeDragId] ?? "ARRIVAL",
                    source: "fake",
                  }
                }
                values={stripValues[activeDragId] ?? defaultStripValues()}
                assignedStand={stripStandAssignments[activeDragId] ?? ""}
                onOpenMenu={openMenu}
                onRequestQnhUpdate={requestQnhUpdate}
                onToggleAta={toggleAta}
                onOpenFpl={handleOpenFpl}
              />
            </article>
          ) : null}
        </DragOverlay>

        <CommandBar
          onFind={handleOpenFindDialog}
          onNew={() => handleOpenNewDialog()}
          deleteDropId={DELETE_BAY_ID}
          isDraggingStrip={activeDragId !== null}
          departureRunway={departureRunway}
          onDepartureRunwayChange={setDepartureRunway}
          arrivalRunway={arrivalRunway}
          onArrivalRunwayChange={setArrivalRunway}
          airport={selectedAirport}
          airportOptions={AIRPORT_OPTIONS}
          onAirportChange={setSelectedAirport}
        />
      </DndContext>

      <MenuDialog
        menuState={menuState}
        value={menuValue}
        onValueChange={(next) => setMenuValue(next)}
        onEsc={closeMenu}
        onOk={confirmMenu}
        onSubmitValue={submitMenuValue}
        onPreset={(preset) => setMenuValue(preset)}
        airport={selectedAirport}
      />

      <FplClearanceDialog
        stripId={fplStripId}
        data={fplStripId ? stripData[fplStripId] : undefined}
        values={fplStripId ? stripValues[fplStripId] : undefined}
        assignedStand={
          fplStripId ? (stripStandAssignments[fplStripId] ?? "") : ""
        }
        onClose={() => setFplStripId(null)}
        onOpenMenu={(type, stripId, target) =>
          openMenu(type, stripId, undefined, target)
        }
        onToggleRea={toggleRea}
        onToggleDeIce={toggleDeIce}
        onStandChange={updateAssignedStand}
        onSidChange={updateSid}
        onSelectVfr={selectVfr}
        onRnavChange={updateRnav}
        onRadisRemarksChange={updateRadisRemarks}
      />

      <FindDialog
        open={findDialogOpen}
        strips={plannedStripData}
        onOpenChange={setFindDialogOpen}
        onSelectStrip={handleSelectPlannedStrip}
      />

      <NewFplDialog
        open={newFplDialogOpen}
        initialCallsign={newFplInitialCallsign}
        initialStrip={
          newFplInitialStripId ? stripData[newFplInitialStripId] : null
        }
        departureAirport={selectedAirport}
        onOpenChange={setNewFplDialogOpen}
        onCreateStrip={handleCreateStrip}
      />
    </div>
  );
}
