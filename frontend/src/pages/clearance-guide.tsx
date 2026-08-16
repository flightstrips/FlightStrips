import { useState } from "react";
import { WebSocketClient } from "@/api/websocket";
import { Bay, CommunicationType, type FrontendStrip } from "@/api/models";
import { createWebSocketStore } from "@/store/store";
import { WebSocketStoreContext } from "@/store/store-context";
import ClearanceDelivery from "@/routes/ekch/CLX";

const guideFlight: FrontendStrip = {
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
  spoken_callsign: "KLM ONE EIGHT X-RAY",
  stand: "A18",
  capabilities: "SDE2E3FGHIRWY/LB1",
  communication_type: CommunicationType.Voice,
  eobt: "1320",
  tobt: "1325",
  tsat: "1331",
  ctot: "",
  phase: "PREFLIGHT",
  eldt: "",
  bay: Bay.NotCleared,
  release_point: "",
  version: 1,
  sequence: 100,
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

export default function ClearanceGuidePage() {
  const params = new URLSearchParams(window.location.search);
  const cleared = params.get("stage") === "cleared";
  const [store] = useState(() => {
    const guideStore = createWebSocketStore(new WebSocketClient("ws://127.0.0.1/unused"));
    guideStore.setState({
      position: "121.725",
      identifier: "DEL",
      airport: "EKCH",
      callsign: "EKCH_DEL",
      readOnly: false,
      positionAvailable: true,
      controllers: [
        { callsign: "EKCH_DEL", position: "121.725", identifier: "DEL", section: "DEL", owned_sectors: ["DEL"] },
        { callsign: "EKCH_A_GND", position: "121.905", identifier: "AD", section: "AD", owned_sectors: ["AD"] },
      ],
      strips: [{
        ...guideFlight,
        bay: cleared ? Bay.Cleared : Bay.NotCleared,
        owner: cleared ? "121.905" : "121.725",
      }],
      tacticalStrips: [],
      messages: [],
      runwaySetup: { departure: ["22R"], arrival: ["22L"], runway_status: {} },
      transitionAltitude: 5000,
      initialCflByRunway: { "22R": 7000 },
      availableSids: [
        { name: "NEXEN2C", runway: "22R" },
        { name: "SIMEG2C", runway: "22R" },
        { name: "LANGO2C", runway: "22R" },
        { name: "NEXEN1A", runway: "04R" },
      ],
      standAssignments: [],
    });
    return guideStore;
  });

  return (
    <WebSocketStoreContext.Provider value={store}>
      <ClearanceDelivery />
    </WebSocketStoreContext.Provider>
  );
}
