import { useState, type ReactNode } from "react";
import { Strip } from "@/components/strip/Strip";
import { MessageStrip } from "@/components/strip/MessageStrip";
import { WebSocketClient } from "@/api/websocket";
import { Bay, CommunicationType, type FrontendStrip, type MessageReceived, type TacticalStrip } from "@/api/models";
import { createWebSocketStore } from "@/store/store";
import { WebSocketStoreContext } from "@/store/store-context";
import {
  EQUAL_COLUMN_CLASSES,
  GEGW_COLUMN_CLASSES,
  PRODUCTION_BAY_CLASS,
  TOWER_COLUMN_CLASSES,
} from "@/routes/ekch/productionBayLayouts";

type Annotation = {
  label: string;
  targetX: number;
  labelX?: number;
  side?: "top" | "bottom";
};

type GalleryItem = {
  id: string;
  title: string;
  bayTitle: string;
  annotations: Annotation[];
  strip: ReactNode;
};

type ProductionBayContext = {
  route: string;
  columns: readonly string[];
  column: number;
  bayClassName: string;
  align?: "start" | "end";
};

const PRODUCTION_BAY_CONTEXTS: Record<string, ProductionBayContext> = {
  "pre-clearance": { route: "TW/TE · CLRDEL", columns: TOWER_COLUMN_CLASSES, column: 3, bayClassName: PRODUCTION_BAY_CLASS.towerClearanceDelivery, align: "start" },
  "cleared-summary": { route: "CLX · CLEARED", columns: EQUAL_COLUMN_CLASSES, column: 2, bayClassName: PRODUCTION_BAY_CLASS.clxCleared, align: "start" },
  "compact-clearance": { route: "CLX · PUSHBACK", columns: EQUAL_COLUMN_CLASSES, column: 3, bayClassName: PRODUCTION_BAY_CLASS.clxPushback },
  pushback: { route: "GE/GW · PUSHBACK", columns: GEGW_COLUMN_CLASSES, column: 1, bayClassName: PRODUCTION_BAY_CLASS.gegwPushback },
  startup: { route: "GE/GW · STARTUP", columns: GEGW_COLUMN_CLASSES, column: 2, bayClassName: PRODUCTION_BAY_CLASS.gegwStartup },
  "apron-departure": { route: "AA · TWY DEP UPR", columns: EQUAL_COLUMN_CLASSES, column: 2, bayClassName: PRODUCTION_BAY_CLASS.aaTaxiDepartureUpper },
  "tower-departure": { route: "TW/TE · TWY DEP", columns: TOWER_COLUMN_CLASSES, column: 1, bayClassName: PRODUCTION_BAY_CLASS.towerTaxiDeparture },
  "final-arrival": { route: "TW/TE · FINAL", columns: TOWER_COLUMN_CLASSES, column: 0, bayClassName: PRODUCTION_BAY_CLASS.towerFinal },
  "apron-arrival": { route: "AA · RWY ARR", columns: EQUAL_COLUMN_CLASSES, column: 0, bayClassName: PRODUCTION_BAY_CLASS.aaRunwayArrival },
  "compact-flight": { route: "AA · FINAL", columns: EQUAL_COLUMN_CLASSES, column: 0, bayClassName: PRODUCTION_BAY_CLASS.aaFinal },
  "control-zone": { route: "TWR/GND · CONTROLZONE", columns: TOWER_COLUMN_CLASSES, column: 2, bayClassName: PRODUCTION_BAY_CLASS.towerControlZoneWithStartup },
  "tactical-memaid": { route: "TW/TE · TWY DEP", columns: TOWER_COLUMN_CLASSES, column: 1, bayClassName: PRODUCTION_BAY_CLASS.towerTaxiDeparture },
  "tactical-crossing": { route: "TW/TE · TWY DEP", columns: TOWER_COLUMN_CLASSES, column: 1, bayClassName: PRODUCTION_BAY_CLASS.towerTaxiDeparture },
  "tactical-start": { route: "TW/TE · RWY ARR", columns: TOWER_COLUMN_CLASSES, column: 0, bayClassName: PRODUCTION_BAY_CLASS.towerRunwayArrival },
  "tactical-land": { route: "TW/TE · RWY ARR", columns: TOWER_COLUMN_CLASSES, column: 0, bayClassName: PRODUCTION_BAY_CLASS.towerRunwayArrival },
  message: { route: "AA/AD · MESSAGES", columns: EQUAL_COLUMN_CLASSES, column: 0, bayClassName: PRODUCTION_BAY_CLASS.adMessages, align: "start" },
};

const baseFlight: FrontendStrip = {
  callsign: "KLM18X",
  origin: "EKCH",
  destination: "EHAM",
  alternate: "ENGM",
  route: "NEXEN Z703",
  remarks: "",
  runway: "22R",
  squawk: "2147",
  assigned_squawk: "2147",
  sid: "NEXEN2C",
  star: "",
  cleared_altitude: 7000,
  requested_altitude: 35000,
  heading: 220,
  aircraft_type: "B738",
  aircraft_category: "M",
  stand: "A18",
  capabilities: "SDE2E3FGHIRWY/LB1",
  communication_type: CommunicationType.Voice,
  eobt: "1320",
  tobt: "1325",
  tsat: "1331",
  ctot: "",
  phase: "TAXI",
  eldt: "",
  bay: Bay.Cleared,
  release_point: "A3",
  version: 1,
  sequence: 1,
  next_controllers: ["118.100"],
  previous_controllers: [],
  next_display: { frequency: "118.100", label: "TW" },
  owner: "121.905",
  pdc_state: "NONE",
  start_req: false,
  marked: false,
  runway_cleared: false,
  runway_confirmed: false,
  registration: "PH-PJK",
};

function flight(overrides: Partial<FrontendStrip> = {}): FrontendStrip {
  return { ...baseFlight, ...overrides };
}

const tacticalBase: TacticalStrip = {
  id: 1,
  session_id: 1,
  type: "MEMAID",
  bay: Bay.TaxiLwr,
  label: "VEHICLE ON A3",
  aircraft: "SAS1234",
  produced_by: "121.905",
  owner: "121.905",
  marked: false,
  sequence: 1,
  confirmed: false,
  confirmed_by: "",
  created_at: "2026-08-16T08:00:00Z",
};

const message: MessageReceived = {
  id: 1,
  sender: "118.100",
  text: "SAS1234 REQUEST RELEASE",
  is_broadcast: false,
  recipients: ["121.905"],
};

function AnnotatedStrip({ id, title, bayTitle, annotations, children }: { id: string; title: string; bayTitle: string; annotations: Annotation[]; children: ReactNode }) {
  const stripTop = 142;
  const stripBottom = 210;

  return (
    <section data-shot={id} className="relative h-[337px] w-[675px] overflow-hidden border-x-2 border-t-2 border-bay-border bg-bay-panel shadow-sm">
      <div className="bay-col-header">
        <span className="text-[0.94vw] font-bold text-white">{bayTitle}</span>
        <span className="ml-auto text-[0.63vw] font-normal text-slate-300">{title}</span>
      </div>
      <div data-testid={`strip-fixture-${id}`} className="absolute left-0 right-0 top-[142px] p-0.5 shadow-[inset_2px_2px_4px_rgba(0,0,0,0.55)]">{children}</div>
      <svg className="pointer-events-none absolute inset-0 h-full w-full" viewBox="0 0 675 337" preserveAspectRatio="none" aria-hidden="true">
        {annotations.map((annotation) => {
          const startX = 10 + (annotation.targetX / 100) * 655;
          const endX = 10 + ((annotation.labelX ?? annotation.targetX) / 100) * 655;
          const top = annotation.side !== "bottom";
          return (
            <g key={`${annotation.label}-${annotation.targetX}`}>
              <line x1={startX} y1={top ? stripTop : stripBottom} x2={endX} y2={top ? 96 : 270} stroke="#f8fafc" strokeWidth="1.5" />
              <circle cx={startX} cy={top ? stripTop : stripBottom} r="3" fill="#f8fafc" />
            </g>
          );
        })}
      </svg>
      {annotations.map((annotation) => {
        const top = annotation.side !== "bottom";
        return (
          <div
            key={`${annotation.label}-label-${annotation.targetX}`}
            className="absolute z-10 -translate-x-1/2 whitespace-nowrap rounded border border-slate-400 bg-white px-1.5 py-1 text-[10px] font-semibold leading-none text-slate-800 shadow-sm"
            style={{ left: `calc(10px + ${(annotation.labelX ?? annotation.targetX) / 100} * (100% - 20px))`, top: top ? 81 : 270 }}
          >
            {annotation.label}
          </div>
        );
      })}
    </section>
  );
}

function ProductionBayFixture({ item }: { item: GalleryItem }) {
  const context = PRODUCTION_BAY_CONTEXTS[item.id];
  if (!context) throw new Error(`Missing production bay context for ${item.id}`);

  return (
    <main className="h-screen w-screen overflow-hidden bg-bay-border">
      <div className="bay-page-wrapper">
        {context.columns.map((columnClassName, index) => (
          <div key={`${item.id}-column-${index}`} className={columnClassName}>
            {index === context.column && (
              <>
                <div className="bay-col-header justify-between">
                  <span className="text-[0.94vw] font-bold text-white">{item.bayTitle}</span>
                  <span className="text-[0.63vw] font-normal text-slate-300">{context.route}</span>
                </div>
                <section
                  data-shot={item.id}
                  data-layout-source={context.route}
                  className={`${context.bayClassName} ${context.align === "start" ? "justify-start" : "justify-end"}`}
                >
                  <div data-testid={`strip-fixture-${item.id}`} className="w-full">
                    {item.strip}
                  </div>
                </section>
              </>
            )}
          </div>
        ))}
      </div>
    </main>
  );
}

function makeGallery(): GalleryItem[] {
  const owner = "121.905";
  const common = { myPosition: owner, selectable: false };

  return [
    {
      id: "pre-clearance",
      title: "Pre-clearance strip",
      bayTitle: "UNCLEARED",
      annotations: [
        { label: "Callsign", targetX: 13.3 },
        { label: "Destination", targetX: 33.3, labelX: 30 },
        { label: "Stand", targetX: 33.3, labelX: 38, side: "bottom" },
        { label: "EOBT / CTOT", targetX: 50 },
        { label: "TOBT / TSAT", targetX: 70, side: "bottom" },
      ],
      strip: <Strip strip={flight({ bay: Bay.NotCleared })} status="CLR" fullWidth {...common} />,
    },
    {
      id: "cleared-summary",
      title: "Cleared summary strip",
      bayTitle: "CLEARED",
      annotations: [
        { label: "SI", targetX: 4.2 },
        { label: "Callsign", targetX: 21.8 },
        { label: "Destination / Stand", targetX: 41.8, labelX: 39, side: "bottom" },
        { label: "EOBT / CTOT", targetX: 58.4 },
        { label: "TOBT / TSAT", targetX: 78.4, side: "bottom" },
      ],
      strip: <Strip strip={flight()} status="CLROK" {...common} />,
    },
    {
      id: "compact-clearance",
      title: "Compact clearance strip",
      bayTitle: "CLEARANCE",
      annotations: [
        { label: "OB", targetX: 3.4 },
        { label: "Callsign", targetX: 17.6 },
        { label: "A/C type", targetX: 35.6, side: "bottom" },
        { label: "Runway", targetX: 48.1 },
        { label: "SID", targetX: 61.5, side: "bottom" },
        { label: "Stand", targetX: 74.6 },
      ],
      strip: <Strip strip={flight({ bay: Bay.Push })} status="CLX-HALF" {...common} />,
    },
    {
      id: "pushback",
      title: "GE/GW pushback strip",
      bayTitle: "PUSHBACK",
      annotations: [
        { label: "SI", targetX: 3.4 },
        { label: "Callsign / next frequency", targetX: 17.4, labelX: 19 },
        { label: "A/C type / Registration", targetX: 35.2, labelX: 36, side: "bottom" },
        { label: "Stand / release point", targetX: 50.4 },
        { label: "TSAT / CTOT", targetX: 65.7, side: "bottom" },
        { label: "Runway", targetX: 79.2 },
      ],
      strip: <Strip strip={flight({ bay: Bay.Push, release_point: "" })} status="PUSH" fullWidth {...common} />,
    },
    {
      id: "startup",
      title: "GE/GW startup strip",
      bayTitle: "STARTUP",
      annotations: [
        { label: "SI", targetX: 3.4 },
        { label: "Callsign / next frequency", targetX: 17.4, labelX: 19 },
        { label: "A/C type / Registration", targetX: 35.2, labelX: 36, side: "bottom" },
        { label: "Stand / release point", targetX: 50.4 },
        { label: "TSAT / CTOT", targetX: 65.7, side: "bottom" },
        { label: "Runway", targetX: 79.2 },
      ],
      strip: <Strip strip={flight({ bay: Bay.Cleared, release_point: "" })} status="PUSH" {...common} />,
    },
    {
      id: "apron-departure",
      title: "Apron departure-taxi strip",
      bayTitle: "TWY DEP",
      annotations: [
        { label: "SI", targetX: 3.4 },
        { label: "Callsign / next frequency", targetX: 17.5, labelX: 18 },
        { label: "A/C type / Registration", targetX: 36, side: "bottom" },
        { label: "Stand / CTOT", targetX: 51.5 },
        { label: "TWY / HP", targetX: 64, side: "bottom" },
        { label: "Runway", targetX: 77.5 },
      ],
      strip: <Strip strip={flight({ bay: Bay.TaxiLwr })} status="TAXI-DEP" {...common} />,
    },
    {
      id: "tower-departure",
      title: "Tower and ground departure strip",
      bayTitle: "TWY DEP",
      annotations: [
        { label: "SI", targetX: 4 },
        { label: "Callsign / next frequency", targetX: 16, labelX: 18 },
        { label: "A/C type / Squawk", targetX: 34, side: "bottom" },
        { label: "Stand", targetX: 46 },
        { label: "TWY", targetX: 56, side: "bottom" },
        { label: "Runway / HP", targetX: 66 },
        { label: "Cleared FL / Heading", targetX: 76, side: "bottom" },
        { label: "SID / Destination / CTOT", targetX: 89, labelX: 90 },
      ],
      strip: <Strip strip={flight({ bay: Bay.TaxiLwr })} status="TWY-DEP" {...common} />,
    },
    {
      id: "final-arrival",
      title: "Final, runway and taxi-arrival strip",
      bayTitle: "FINAL",
      annotations: [
        { label: "SI", targetX: 4 },
        { label: "Callsign / next frequency", targetX: 19, labelX: 20 },
        { label: "A/C type / Squawk", targetX: 39, side: "bottom" },
        { label: "Stand", targetX: 55 },
        { label: "Runway / HP or TWY", targetX: 68, side: "bottom" },
        { label: "Flight plan", targetX: 84 },
      ],
      strip: <Strip strip={flight({ origin: "ESSA", destination: "EKCH", bay: Bay.Final, runway: "04L", sid: "", release_point: "" })} status="FINAL-ARR" {...common} />,
    },
    {
      id: "apron-arrival",
      title: "Apron arrival strip",
      bayTitle: "TWY ARR",
      annotations: [
        { label: "SI", targetX: 4.1 },
        { label: "Callsign / next frequency", targetX: 20.5, labelX: 21 },
        { label: "A/C type / Registration", targetX: 41, side: "bottom" },
        { label: "Runway", targetX: 55.5 },
        { label: "TWY", targetX: 66.5, side: "bottom" },
        { label: "Stand", targetX: 81.5 },
      ],
      strip: <Strip strip={flight({ origin: "ESSA", destination: "EKCH", bay: Bay.Stand, sid: "" })} status="ARR" {...common} />,
    },
    {
      id: "compact-flight",
      title: "Compact flight strip",
      bayTitle: "PUSHBACK",
      annotations: [
        { label: "Identifier", targetX: 3.5 },
        { label: "Callsign", targetX: 18 },
        { label: "A/C type", targetX: 38, side: "bottom" },
        { label: "Runway", targetX: 49.5 },
        { label: "TWY", targetX: 60.5, side: "bottom" },
        { label: "HP", targetX: 69.5 },
        { label: "Stand", targetX: 76.5, side: "bottom" },
      ],
      strip: <Strip strip={flight({ origin: "ESSA", destination: "EKCH", bay: Bay.Final, sid: "" })} status="HALF" halfStripVariant="LOCKED-ARR" {...common} />,
    },
    {
      id: "control-zone",
      title: "Control-zone strip",
      bayTitle: "CONTROLZONE",
      annotations: [
        { label: "Indicator", targetX: 3.2 },
        { label: "Callsign / Remarks", targetX: 18, labelX: 20 },
        { label: "Squawk", targetX: 37, side: "bottom" },
        { label: "QNH / POB", targetX: 50.5 },
        { label: "Status / Language / FPL type", targetX: 62, labelX: 64, side: "bottom" },
        { label: "Flight plan", targetX: 79.5 },
      ],
      strip: <Strip strip={flight({ callsign: "OYABC", bay: Bay.Controlzone, remarks: "LOCAL VFR", persons_on_board: 3, language: "DA", fpl_type: "VFR", position_altitude: 350 })} status="CONTROLZONE" {...common} />,
    },
    {
      id: "tactical-memaid",
      title: "Tactical memory-aid strip",
      bayTitle: "MEMORY AIDS",
      annotations: [
        { label: "Ownership", targetX: 4 },
        { label: "Tactical text", targetX: 45 },
        { label: "Confirmation", targetX: 83, side: "bottom" },
        { label: "Delete", targetX: 93 },
      ],
      strip: <Strip strip={tacticalBase} width="95%" {...common} />,
    },
    {
      id: "tactical-crossing",
      title: "Tactical crossing strip",
      bayTitle: "CROSSINGS",
      annotations: [
        { label: "Ownership", targetX: 4 },
        { label: "Crossing", targetX: 45 },
        { label: "Delete", targetX: 93 },
      ],
      strip: <Strip strip={{ ...tacticalBase, id: 2, type: "CROSSING", label: "CROSS 04L", aircraft: "OY-ABC" }} width="95%" {...common} />,
    },
    {
      id: "tactical-start",
      title: "Tactical runway start strip",
      bayTitle: "RUNWAY",
      annotations: [
        { label: "Ownership", targetX: 4 },
        { label: "Start clearance", targetX: 45 },
        { label: "Delete", targetX: 93 },
      ],
      strip: <Strip strip={{ ...tacticalBase, id: 3, type: "START", label: "04L", aircraft: "SAS1234" }} width="95%" {...common} />,
    },
    {
      id: "tactical-land",
      title: "Tactical runway landing strip",
      bayTitle: "RUNWAY",
      annotations: [
        { label: "Ownership", targetX: 4 },
        { label: "Landing clearance", targetX: 45 },
        { label: "Delete", targetX: 93 },
      ],
      strip: <Strip strip={{ ...tacticalBase, id: 4, type: "LAND", label: "04L", aircraft: "KLM18X" }} width="95%" {...common} />,
    },
    {
      id: "message",
      title: "Message strip",
      bayTitle: "MESSAGES",
      annotations: [
        { label: "Sender", targetX: 2 },
        { label: "Message text", targetX: 45 },
        { label: "Dismiss", targetX: 97 },
      ],
      strip: <div className="w-[95%]"><MessageStrip msg={message} /></div>,
    },
  ];
}

export default function StripGalleryPage() {
  const [store] = useState(() => {
    const nextStore = createWebSocketStore(new WebSocketClient("ws://127.0.0.1/unused"));
    const galleryFlights = [baseFlight];
    nextStore.setState({
      position: "121.905",
      identifier: "AD",
      airport: "EKCH",
      callsign: "EKCH_A_GND",
      controllers: [
        { callsign: "EKCH_A_GND", position: "121.905", identifier: "AD", section: "AD", owned_sectors: ["AD"] },
        { callsign: "EKCH_TWR", position: "118.100", identifier: "TW", section: "TW", owned_sectors: ["TW"] },
      ],
      strips: galleryFlights,
      tacticalStrips: [tacticalBase],
      runwaySetup: { departure: ["22R"], arrival: ["22L"], runway_status: {} },
      metar: "EKCH 160950Z 24008KT CAVOK 18/10 Q1013",
      transitionAltitude: 5000,
      initialCflByRunway: { "22R": 7000 },
      standAssignments: [],
    });
    return nextStore;
  });
  const gallery = makeGallery();
  const requestedShot = new URLSearchParams(window.location.search).get("shot");
  const visibleGallery = requestedShot ? gallery.filter((item) => item.id === requestedShot) : gallery;

  if (requestedShot) {
    const item = visibleGallery[0];
    if (!item) return <main>Unknown strip fixture: {requestedShot}</main>;
    return (
      <WebSocketStoreContext.Provider value={store}>
        <ProductionBayFixture item={item} />
      </WebSocketStoreContext.Provider>
    );
  }

  return (
    <WebSocketStoreContext.Provider value={store}>
      <main className={`min-h-screen bg-slate-200 text-slate-900 ${requestedShot ? "p-0" : "p-8"}`}>
        <div className={`grid w-fit ${requestedShot ? "gap-0" : "mx-auto gap-6"}`}>
          {!requestedShot && (
            <header className="w-[675px] rounded border border-slate-300 bg-white p-5">
              <h1 className="text-2xl font-bold">Flight Strips documentation capture gallery</h1>
              <p className="mt-1 text-sm text-slate-600">Local-development-only page. Each panel is captured into the Concepts documentation.</p>
            </header>
          )}
          {visibleGallery.map((item) => (
            <AnnotatedStrip key={item.id} id={item.id} title={item.title} bayTitle={item.bayTitle} annotations={item.annotations}>
              {item.strip}
            </AnnotatedStrip>
          ))}
        </div>
      </main>
    </WebSocketStoreContext.Provider>
  );
}
