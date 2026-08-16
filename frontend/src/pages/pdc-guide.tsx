import { useEffect, useState } from "react";
import { WebSocketClient } from "@/api/websocket";
import { Bay, CommunicationType, type FrontendStrip, type PdcStatus } from "@/api/models";
import { Strip } from "@/components/strip/Strip";
import { createWebSocketStore } from "@/store/store";
import { WebSocketStoreContext } from "@/store/store-context";

const baseFlight: FrontendStrip = {
  callsign: "KLM18X",
  origin: "EKCH",
  destination: "EHAM",
  alternate: "EHRD",
  route: "NEXEN Z703 TOSVA",
  remarks: "PBN/A1B1C1D1 DOF/260816",
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
  communication_type: CommunicationType.Text,
  eobt: "1320",
  tobt: "1325",
  tsat: "1331",
  ctot: "",
  phase: "PREFLIGHT",
  eldt: "",
  bay: Bay.NotCleared,
  release_point: "",
  version: 1,
  sequence: 1,
  next_controllers: ["121.905"],
  previous_controllers: [],
  next_display: { frequency: "121.905", label: "AD" },
  owner: "121.725",
  pdc_state: "NONE",
  start_req: false,
  marked: false,
  runway_cleared: false,
  runway_confirmed: false,
  registration: "PH-PJK",
};

function ClearedBlinkStrip() {
  const [pdcState, setPdcState] = useState<PdcStatus>("REQUESTED");

  useEffect(() => {
    const timer = window.setTimeout(() => setPdcState("CLEARED"), 1000);
    return () => window.clearTimeout(timer);
  }, []);

  return <Strip strip={{ ...baseFlight, pdc_state: pdcState }} status="CLR" myPosition="121.725" selectable={false} />;
}

export default function PdcGuidePage() {
  const shot = new URLSearchParams(window.location.search).get("shot") ?? "validation";
  const validationFlight: FrontendStrip = {
    ...baseFlight,
    pdc_state: "REQUESTED_WITH_FAULTS",
    validation_status: {
      issue_type: "PDC INVALID",
      message: "Pilot requested PDC, but the clearance is invalid:\n• Runway 04R is not an active departure runway\nOpen DCL menu to review the request and correct the issue.",
      owning_position: "121.725",
      active: true,
      activation_key: "pdc-guide",
      custom_action: { label: "OPEN DCL MENU", action_kind: "open_dcl_menu" },
    },
  };
  const [store] = useState(() => {
    const guideStore = createWebSocketStore(new WebSocketClient("ws://127.0.0.1/unused"));
    guideStore.setState({
      position: "121.725",
      identifier: "DEL",
      airport: "EKCH",
      callsign: "EKCH_DEL",
      controllers: [
        { callsign: "EKCH_DEL", position: "121.725", identifier: "DEL", section: "DEL", owned_sectors: ["DEL"] },
      ],
      strips: shot === "validation" ? [validationFlight] : [baseFlight],
      runwaySetup: { departure: ["22R"], arrival: ["22L"], runway_status: {} },
      transitionAltitude: 5000,
      initialCflByRunway: { "22R": 7000 },
      standAssignments: [],
    });
    return guideStore;
  });

  return (
    <WebSocketStoreContext.Provider value={store}>
      <main className="min-h-screen bg-[#555357] p-5 text-white">
        <section data-shot={shot} className="w-[760px]">
          <div className="mb-3 text-[18px] font-bold">
            {shot === "validation" ? "PDC NEEDS CONTROLLER ACTION" : "PDC SENT — WAIT FOR PILOT"}
          </div>
          {shot === "validation" ? (
            <Strip strip={validationFlight} status="CLR" myPosition="121.725" selectable={false} />
          ) : (
            <ClearedBlinkStrip />
          )}
        </section>
      </main>
    </WebSocketStoreContext.Provider>
  );
}
