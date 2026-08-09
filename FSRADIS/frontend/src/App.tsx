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

type BayId = "ACTIVE" | "RUNWAY" | "PASSIVE_TOP" | "PASSIVE_BOTTOM";
type MenuType = "RTE" | "ALT" | "NUM" | "FPL" | "RWY";
type NumField = "B5" | "B6" | "B7";
type StripType = "ARRIVAL" | "DEPARTURE";
const DELETE_BAY_ID = "DELETE_BAY";

const TARGET_AIRPORT = "EKYT";
const VATSIM_DATA_URL = "https://data.vatsim.net/v3/vatsim-data.json";

interface VatsimPilot {
  callsign: string;
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
  };
}

interface VatsimDataResponse {
  pilots?: VatsimPilot[];
  prefiles?: VatsimPilot[];
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
  source: "vatsim" | "fake" | "manual";
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
  stripType: StripType;
  source: "vatsim" | "fake" | "manual";
}

const STATIC_FAKE_FLIGHTPLANS: FlightPlanSeed[] = [
  {
    callsign: "RYR2AA",
    departure: "EKYT",
    arrival: "EKCH",
    aircraft: "B738",
    wakeTurbulence: "M",
    route: "AAL ADSEN",
    altitude: "FL230",
    depTime: "1215",
    hrsEnroute: "00",
    minEnroute: "35",
    assignedSquawk: "4251",
    source: "fake",
  },
  {
    callsign: "SAS7LK",
    departure: "EKYT",
    arrival: "EKCH",
    aircraft: "A320",
    wakeTurbulence: "M",
    route: "L983 PTH",
    altitude: "FL190",
    depTime: "1230",
    hrsEnroute: "00",
    minEnroute: "28",
    assignedSquawk: "4312",
    source: "fake",
  },
  {
    callsign: "DAL4MX",
    departure: "EGPD",
    arrival: "EKYT",
    aircraft: "B752",
    wakeTurbulence: "H",
    route: "N560 P607",
    altitude: "FL310",
    depTime: "1140",
    hrsEnroute: "01",
    minEnroute: "12",
    assignedSquawk: "5064",
    source: "fake",
  },
  {
    callsign: "KLM88N",
    departure: "EHAM",
    arrival: "EKYT",
    aircraft: "E190",
    wakeTurbulence: "M",
    route: "N873 P60",
    altitude: "FL240",
    depTime: "1155",
    hrsEnroute: "01",
    minEnroute: "04",
    assignedSquawk: "3720",
    source: "fake",
  },
];

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
}

interface MenuState {
  type: MenuType;
  stripId: string;
  field?: NumField;
}

interface SortablePrototypeStripProps {
  stripId: string;
  stripType: StripType;
  displayData: StripDisplayData;
  stripStyle: CSSProperties;
  displayDate: string;
  values: StripValues;
  onOpenMenu: (type: MenuType, stripId: string, field?: NumField) => void;
  onRequestQnhUpdate: (stripId: string) => void;
}

interface PrototypeStripSectionsProps {
  displayDate: string;
  stripId: string;
  stripType: StripType;
  displayData: StripDisplayData;
  values: StripValues;
  onOpenMenu: (type: MenuType, stripId: string, field?: NumField) => void;
  onRequestQnhUpdate: (stripId: string) => void;
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
  onOpenMenu: (type: MenuType, stripId: string, field?: NumField) => void;
  onRequestQnhUpdate: (stripId: string) => void;
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
  onOpenMenu,
  onRequestQnhUpdate,
  reverse = false,
}: SortableStripListProps) {
  const { setNodeRef, isOver } = useDroppable({ id: bayId });

  return (
    <div
      ref={setNodeRef}
      className={`strip-list strip-drop-zone ${reverse ? "reverse-order" : ""} ${isOver ? "is-drop-over" : ""}`}
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
                departure: TARGET_AIRPORT,
                arrival: TARGET_AIRPORT,
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
              }
            }
            onOpenMenu={onOpenMenu}
            onRequestQnhUpdate={onRequestQnhUpdate}
          />
        ))}
      </SortableContext>
    </div>
  );
}

function MenuDialog({
  menuState,
  value,
  onValueChange,
  onEsc,
  onOk,
  onSubmitValue,
  onPreset,
}: {
  menuState: MenuState | null;
  value: string;
  onValueChange: (next: string) => void;
  onEsc: () => void;
  onOk: () => void;
  onSubmitValue: (nextValue: string) => void;
  onPreset: (preset: string) => void;
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
  const runwayPairs = [
    { left: "08L", right: "26R" },
    { left: "08R", right: "26L" },
  ];
  const altOptions = ["F130", "F120", "F70", "F50", "F40", "F30", "F23", "F20", "F15"];
  const pointOptions = {
    left: ["TOSVI", "GIPUG", "KUDEV"],
    right: ["UPZIW", "BAKIT", "EWTIQ"],
    center: "AAL",
  };

  function handlePointSelect(point: string) {
    onSubmitValue(point);
    setPointDialogOpen(false);
  }

  function handleRunwaySelect(runway: string) {
    onSubmitValue(runway);
  }

  if (isRte && pointDialogOpen) {
    return (
      <div className="menu-overlay" role="dialog" aria-modal="true" aria-label="Point Dialogue">
        <div className="point-menu-frame">
          <div className="point-menu-inner">
            <div className="point-menu-row point-menu-row-top">
              <button className="point-menu-box" onClick={() => handlePointSelect(pointOptions.left[0])}>{pointOptions.left[0]}</button>
              <button className="point-menu-box" onClick={() => handlePointSelect(pointOptions.right[0])}>{pointOptions.right[0]}</button>
            </div>

            <div className="point-menu-center-row" aria-hidden>
              <div className="point-menu-line-group point-menu-line-group-left">
                <div className="point-menu-line" />
                <div className="point-menu-line" />
              </div>
              <div className="point-menu-line-main" />
              <button className="point-menu-box point-menu-box-center" onClick={() => handlePointSelect(pointOptions.center)}>{pointOptions.center}</button>
              <div className="point-menu-line-group point-menu-line-group-right">
                <div className="point-menu-line" />
                <div className="point-menu-line" />
              </div>
            </div>

            <div className="point-menu-row point-menu-row-mid">
              <button className="point-menu-box" onClick={() => handlePointSelect(pointOptions.left[1])}>{pointOptions.left[1]}</button>
              <button className="point-menu-box" onClick={() => handlePointSelect(pointOptions.right[1])}>{pointOptions.right[1]}</button>
            </div>

            <div className="point-menu-row point-menu-row-low">
              <button className="point-menu-box" onClick={() => handlePointSelect(pointOptions.left[2])}>{pointOptions.left[2]}</button>
              <button className="point-menu-box" onClick={() => handlePointSelect(pointOptions.right[2])}>{pointOptions.right[2]}</button>
            </div>

            <button className="point-menu-esc" onClick={onEsc}>ESC</button>
          </div>
        </div>
      </div>
    );
  }

  if (isRwy) {
    return (
      <div className="menu-overlay" role="dialog" aria-modal="true" aria-label="RWY Menu">
        <div className="menu-panel menu-panel-rwy">
          <div className="menu-title">RWY MENU</div>

          <div className="menu-grid menu-grid-rwy">
            {runwayPairs.map((pair, index) => (
              <div key={`${pair.left}-${pair.right}`} className={`menu-rwy-row menu-rwy-row-${index + 1}`}>
                <button className="menu-rwy-box" onClick={() => handleRunwaySelect(pair.left)}>{pair.left}</button>
                <div className="menu-rwy-line" aria-hidden />
                <button className="menu-rwy-box" onClick={() => handleRunwaySelect(pair.right)}>{pair.right}</button>
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

            <button className="menu-key menu-key-point" onClick={() => setPointDialogOpen(true)}>
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
            {["7", "8", "9", "4", "5", "6", "1", "2", "3", ".", "0", "/"].map((keyValue) => (
              <button key={keyValue} className="menu-key" onClick={() => onValueChange(`${value}${keyValue}`)}>
                {keyValue}
              </button>
            ))}
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
            onChange={(event) => onValueChange(event.target.value.toUpperCase())}
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
  onOpenMenu,
  onRequestQnhUpdate,
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
          <div className="box-1-middle-stack" aria-label="First two route points">
            <span className="box-1-middle-stack-line">{displayData.firstWaypoint}</span>
            <span className="box-1-middle-stack-line">{displayData.firstAirwayOrSecondPoint}</span>
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
        <button className="box-cell box-cell-bold box-cell-clickable" onClick={() => onOpenMenu("RWY", stripId)} aria-label="Open RWY MENU">
          {values.U2 || "26R"}
        </button>
        <button className="box-cell box-cell-bold" onClick={() => onRequestQnhUpdate(stripId)}>
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
          className="box-cell box-cell-clickable box-cell-right box-cell-altitude-entered"
          onClick={() => onOpenMenu("ALT", stripId)}
          aria-label="Open ALT MENU bottom"
        >
          {values.B4}
        </button>
      </section>

      <section className="strip-box box-5 split-borders">
        <div className="labeled-cell labeled-cell-static" aria-label="EOBT static">
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
        <div className="labeled-cell labeled-cell-static" aria-label="EET static">
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
        <div className="labeled-cell labeled-cell-static" aria-label="ETA static">
          <span className="labeled-cell-header">ETA</span>
          <span className="labeled-cell-value">{values.U7}</span>
        </div>
        <button
          className="labeled-cell box-cell-clickable"
          onClick={() => onOpenMenu("NUM", stripId, "B7")}
          aria-label="Open NUM MENU ATA"
        >
          <span className="labeled-cell-header">ATA</span>
          <span className="labeled-cell-value">{values.B7}</span>
        </button>
      </section>

      <button
        className="strip-box box-8 box-cell-clickable"
        onClick={() => onOpenMenu("FPL", stripId)}
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
              aria-label={stripType === "DEPARTURE" ? "Departure icon" : "Arrival icon"}
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
          <span className="labeled-cell-value">X</span>
        </div>
        <div className="box-8-bottom box-8-bottom-middle" />
        <div className="box-8-bottom box-8-bottom-right">
          <span className="ap-text">{TARGET_AIRPORT.slice(-2)}</span>
        </div>
      </button>

      <section className="strip-box box-9" aria-label="Date">
        <span className="date-rotated">{displayDate}</span>
      </section>
    </>
  );
}

function SortablePrototypeStrip({
  stripId,
  stripType,
  displayData,
  stripStyle,
  displayDate,
  values,
  onOpenMenu,
  onRequestQnhUpdate,
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
        onOpenMenu={onOpenMenu}
        onRequestQnhUpdate={onRequestQnhUpdate}
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
  };
}

function normalizeCallsign(value: string): string {
  return value.trim().toUpperCase();
}

function normalizeIcao(value: string | undefined): string {
  return (value ?? "").trim().toUpperCase();
}

function normalizeDigits(value: string | undefined): string {
  return (value ?? "").replace(/\D/g, "");
}

function normalizeEnrouteTime(value: string | undefined): { hrsEnroute: string; minEnroute: string } {
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

function parseRouteSegments(route: string): { firstWaypoint: string; firstAirwayOrSecondPoint: string } {
  const segments = route
    .trim()
    .toUpperCase()
    .split(/\s+/)
    .filter((segment) => segment.length > 0 && segment !== "DCT");

  return {
    firstWaypoint: segments[0] ?? "",
    firstAirwayOrSecondPoint: segments[1] ?? "",
  };
}

function addMinutesToHHMM(hhmm: string, minutesToAdd: number): string {
  const normalized = normalizeDigits(hhmm).padStart(4, "0").slice(-4);
  const hours = Number.parseInt(normalized.slice(0, 2), 10);
  const minutes = Number.parseInt(normalized.slice(2, 4), 10);
  const totalMinutes = ((hours * 60) + minutes + minutesToAdd + (24 * 60)) % (24 * 60);
  const outHours = Math.floor(totalMinutes / 60);
  const outMinutes = totalMinutes % 60;

  return `${String(outHours).padStart(2, "0")}${String(outMinutes).padStart(2, "0")}`;
}

function classifyStripType(departure: string, arrival: string): StripType | null {
  if (arrival === TARGET_AIRPORT) {
    return "ARRIVAL";
  }
  if (departure === TARGET_AIRPORT) {
    return "DEPARTURE";
  }
  return null;
}

function toDisplayData(seed: FlightPlanSeed): StripDisplayData | null {
  const departure = normalizeIcao(seed.departure);
  const arrival = normalizeIcao(seed.arrival);
  const stripType = classifyStripType(departure, arrival);
  if (!stripType) {
    return null;
  }

  const route = seed.route.trim().toUpperCase();
  const routeSegments = parseRouteSegments(route);

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
    stripType,
    source: seed.source,
  };
}

function buildStripValuesFromDisplayData(data: StripDisplayData): StripValues {
  const plannedArrival = addMinutesToHHMM(
    data.depTime,
    (Number.parseInt(data.hrsEnroute || "0", 10) * 60)
      + Number.parseInt(data.minEnroute || "0", 10)
      + 15,
  );

  return {
    // 2/U2: departure runway is currently static and should later come from EuroScope/FlightStrips backend.
    U2: "26R",
    // 2/B2: starts as QNH prompt; live METAR-derived value should be written by QNH update flow.
    B2: "QNH",
    // 3/U3 and 3/B3: both fields default to blank for now by requirement.
    U3: "",
    B3: "",
    // 4/U4: altitude shown top-left, bold, same visual scale as 5-7 values.
    U4: data.altitude,
    B4: "",
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
  };
}

function buildStripDataFromSeeds(seeds: FlightPlanSeed[]): Record<string, StripDisplayData> {
  const result: Record<string, StripDisplayData> = {};

  seeds.forEach((seed, index) => {
    const data = toDisplayData(seed);
    if (!data) {
      return;
    }
    result[`${data.stripType === "DEPARTURE" ? "dep" : "arr"}-strip-${index + 1}`] = data;
  });

  return result;
}

function buildBaysFromStripData(data: Record<string, StripDisplayData>): BayState {
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
    ACTIVE: arrivals,
    RUNWAY: [],
    PASSIVE_TOP: departures.filter((_, index) => index % 2 === 0),
    PASSIVE_BOTTOM: departures.filter((_, index) => index % 2 === 1),
  };
}

function buildTypesFromStripData(data: Record<string, StripDisplayData>): Record<string, StripType> {
  const result: Record<string, StripType> = {};
  Object.entries(data).forEach(([id, strip]) => {
    result[id] = strip.stripType;
  });
  return result;
}

async function fetchEKYTFlightPlans(): Promise<FlightPlanSeed[]> {
  const response = await fetch(VATSIM_DATA_URL);
  if (!response.ok) {
    throw new Error(`VATSIM feed request failed: ${response.status}`);
  }

  const payload = (await response.json()) as VatsimDataResponse;
  const pilots = [...(payload.pilots ?? []), ...(payload.prefiles ?? [])];

  const flights = pilots.reduce<FlightPlanSeed[]>((acc, pilot) => {
      const departure = normalizeIcao(pilot.flight_plan?.departure);
      const arrival = normalizeIcao(pilot.flight_plan?.arrival);
      const stripType = classifyStripType(departure, arrival);
      if (!pilot.callsign || !stripType) {
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
        source: "vatsim" as const,
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

  const [stripData, setStripData] = useState<Record<string, StripDisplayData>>(() => {
    const seeded = buildStripDataFromSeeds(STATIC_FAKE_FLIGHTPLANS);
    return seeded;
  });

  const [runwayHeight, setRunwayHeight] = useState(210);
  const [stripHeightPx, setStripHeightPx] = useState(
    () => window.screen.height * 0.08,
  );
  const [activeDragId, setActiveDragId] = useState<string | null>(null);
  const [menuState, setMenuState] = useState<MenuState | null>(null);
  const [menuValue, setMenuValue] = useState("");
  const [bays, setBays] = useState<BayState>(() => buildBaysFromStripData(stripData));
  const [stripValues, setStripValues] = useState<Record<string, StripValues>>(() => {
    const initial: Record<string, StripValues> = {};
    Object.entries(stripData).forEach(([id, data]) => {
      initial[id] = buildStripValuesFromDisplayData(data);
    });
    return initial;
  });
  const [hiddenCallsigns, setHiddenCallsigns] = useState<Set<string>>(() => new Set());
  const [findDialogOpen, setFindDialogOpen] = useState(false);
  const [newFplDialogOpen, setNewFplDialogOpen] = useState(false);
  const [newFplInitialCallsign, setNewFplInitialCallsign] = useState("");
  const [newFplInitialStripId, setNewFplInitialStripId] = useState<string | null>(null);
  const [departureRunway, setDepartureRunway] = useState("22R");
  const [arrivalRunway, setArrivalRunway] = useState("22L");

  const visibleStripData = Object.fromEntries(
    Object.entries(stripData).filter(([, data]) => !hiddenCallsigns.has(data.callsign.toUpperCase())),
  ) as Record<string, StripDisplayData>;
  const visibleStripTypes = buildTypesFromStripData(visibleStripData);

  useEffect(() => {
    setBays(buildBaysFromStripData(visibleStripData));
  }, [hiddenCallsigns, stripData]);

  useEffect(() => {
    let active = true;

    async function loadFlightPlans() {
      try {
        const vatsimSeeds = await fetchEKYTFlightPlans();
        const merged = vatsimSeeds.length > 0
          ? [...vatsimSeeds, ...STATIC_FAKE_FLIGHTPLANS]
          : STATIC_FAKE_FLIGHTPLANS;
        const nextData = buildStripDataFromSeeds(merged);

        if (!active || Object.keys(nextData).length === 0) {
          return;
        }

        setStripData(nextData);
        setStripValues((current) => {
          const next = { ...current };
          Object.entries(nextData).forEach(([stripId, data]) => {
            if (!next[stripId]) {
              next[stripId] = buildStripValuesFromDisplayData(data);
            }
          });
          return next;
        });
      } catch {
        // Keep static fake plans when VATSIM is unavailable.
      }
    }

    loadFlightPlans();

    const intervalId = window.setInterval(loadFlightPlans, 60_000);
    return () => {
      active = false;
      window.clearInterval(intervalId);
    };
  }, []);

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
    fetch(`https://metar.vatsim.net/${TARGET_AIRPORT}`)
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

  function handleOpenFindDialog() {
    setFindDialogOpen(true);
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

    const existingEntry = Object.entries(stripData).find(([, strip]) => normalizeCallsign(strip.callsign) === normalized);
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
    const displayData = toDisplayData(seed);
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
      [stripId]: buildStripValuesFromDisplayData(displayData),
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

  function openMenu(type: MenuType, stripId: string, field?: NumField) {
    setMenuState({ type, stripId, field });

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

        return {
          ...current,
          [menuState.stripId]: {
            ...strip,
            U2: cleaned,
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
      itemId === "ACTIVE"
      || itemId === "RUNWAY"
      || itemId === "PASSIVE_TOP"
      || itemId === "PASSIVE_BOTTOM"
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
          id === "ACTIVE"
          || id === "RUNWAY"
          || id === "PASSIVE_TOP"
          || id === "PASSIVE_BOTTOM"
          || id === DELETE_BAY_ID
        );
      }),
    });

    if (bayCollisions.length > 0) {
      return bayCollisions;
    }

    return closestCenter(args);
  }) satisfies CollisionDetection;

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        delay: 160,
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
                onOpenMenu={openMenu}
                onRequestQnhUpdate={requestQnhUpdate}
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
                onOpenMenu={openMenu}
                onRequestQnhUpdate={requestQnhUpdate}
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
                    onOpenMenu={openMenu}
                    onRequestQnhUpdate={requestQnhUpdate}
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
                    onOpenMenu={openMenu}
                    onRequestQnhUpdate={requestQnhUpdate}
                    reverse
                  />
                </div>
              </div>
            </article>
          </section>
        </main>

        <DragOverlay>
          {activeDragId ? (
            <article className="flight-strip flight-strip-overlay" style={stripStyle}>
              <PrototypeStripSections
                displayDate={displayDate}
                stripId={activeDragId}
                stripType={visibleStripTypes[activeDragId] ?? "ARRIVAL"}
                displayData={
                  stripData[activeDragId] ?? {
                    callsign: "N/A",
                    departure: TARGET_AIRPORT,
                    arrival: TARGET_AIRPORT,
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
                onOpenMenu={openMenu}
                onRequestQnhUpdate={requestQnhUpdate}
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
      />

      <FindDialog
        open={findDialogOpen}
        strips={stripData}
        hiddenCallsigns={hiddenCallsigns}
        onOpenChange={setFindDialogOpen}
        onOpenExisting={handleOpenExistingStrip}
        onRequestNew={handleRequestNewFromFind}
        onHideCallsign={(callsign) => {
          const normalized = normalizeCallsign(callsign);
          setHiddenCallsigns((current) => {
            if (current.has(normalized)) {
              return current;
            }

            const next = new Set(current);
            next.add(normalized);
            return next;
          });
        }}
      />

      <NewFplDialog
        open={newFplDialogOpen}
        initialCallsign={newFplInitialCallsign}
        initialStrip={newFplInitialStripId ? stripData[newFplInitialStripId] : null}
        onOpenChange={setNewFplDialogOpen}
        onCreateStrip={handleCreateStrip}
      />
    </div>
  );
}
