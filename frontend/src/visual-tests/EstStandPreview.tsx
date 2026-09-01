import { createRoot } from "react-dom/client";

/* eslint-disable react-refresh/only-export-components -- isolated Playwright fixture entry point */

import "@/index.css";
import { Bay, type FrontendStandAssignmentEntry, type FrontendStrip } from "@/api/models";
import EstStandCell from "@/components/est/EstStandCell";
import { deriveEstStandDisplay } from "@/components/est/standDisplay";

function makeArrival(callsign: string, stand: string, bay: Bay): FrontendStrip {
  return {
    callsign,
    origin: "ESSA",
    destination: "EKCH",
    alternate: "",
    route: "NILUG N850 LUNIP",
    remarks: "",
    runway: "22L",
    squawk: "1000",
    assigned_squawk: "1000",
    sid: "",
    star: "NILUG2B",
    cleared_altitude: 0,
    requested_altitude: 0,
    heading: 0,
    aircraft_type: "A320/H-SDE2E3FGHIRWXY",
    aircraft_category: "",
    stand,
    capabilities: "",
    communication_type: "",
    eobt: "",
    tobt: "",
    tsat: "",
    ctot: "",
    eldt: "",
    bay,
    release_point: "",
    version: 1,
    sequence: 0,
    next_controllers: [],
    previous_controllers: [],
    owner: "",
    pdc_state: "NONE",
    start_req: false,
    marked: false,
    runway_cleared: false,
    runway_confirmed: false,
    registration: "",
  } as FrontendStrip;
}

const assignedInbound = makeArrival("SAS401", "A18", Bay.Final);
const parkedArrival = makeArrival("NAX782", "A19", Bay.Stand);
const reservedDeparture = makeArrival("SAS900", "A20", Bay.DepHidden);
const arrivalAssignments: FrontendStandAssignmentEntry[] = [
  { callsign: "SAS401", stand: "A18", direction: "ARRIVAL", stage: "CONFIRMED", source: "AUTOMATIC" },
  { callsign: "NAX782", stand: "A19", direction: "ARRIVAL", stage: "CONFIRMED", source: "AUTOMATIC" },
  { callsign: "SAS900", stand: "A20", direction: "DEPARTURE", stage: "RESERVED", source: "AUTOMATIC" },
];
const display = deriveEstStandDisplay([assignedInbound, parkedArrival, reservedDeparture], arrivalAssignments, true);

function PreviewCell({ stand }: { stand: string }) {
  return (
    <EstStandCell
      stand={{ label: stand }}
      strip={display.stripsByStand.get(stand)}
      selected={false}
      blocked={false}
      actionActive={false}
      blinking={false}
      startReqActive={false}
      ctotImproved={false}
      nowMs={Date.UTC(2026, 8, 1, 18, 0, 0)}
      onClick={() => {}}
    />
  );
}

function EstStandPreview() {
  return (
    <main className="min-h-screen bg-[#767676] p-10 text-white">
      <h1 className="mb-2 text-2xl font-bold">EST arrival stand visibility</h1>
      <p className="mb-8 text-sm">Isolated browser fixture — no backend, authentication, or WebSocket connection.</p>
      <div data-testid="est-arrival-visual" className="flex w-fit gap-12 rounded-xl bg-[#555355] p-8">
        <section data-testid="assigned-inbound" className="flex flex-col items-center gap-3">
          <h2 className="font-semibold">Assigned inbound</h2>
          <PreviewCell stand="A18" />
          <p className="max-w-44 text-center text-xs">No arrival information before physical occupancy</p>
        </section>
        <section data-testid="parked-arrival" className="flex flex-col items-center gap-3">
          <h2 className="font-semibold">Physically on stand</h2>
          <PreviewCell stand="A19" />
          <p className="max-w-44 text-center text-xs">Yellow with callsign and aircraft type after entering STAND</p>
        </section>
        <section data-testid="reserved-departure" className="flex flex-col items-center gap-3">
          <h2 className="font-semibold">Reserved departure</h2>
          <PreviewCell stand="A20" />
          <p className="max-w-44 text-center text-xs">No SAT reservation information before physical occupancy</p>
        </section>
      </div>
    </main>
  );
}

createRoot(document.getElementById("root")!).render(<EstStandPreview />);
