import { createRoot } from "react-dom/client";
import { ApnArrStrip } from "@/components/strip/ApnArrStrip";
import { ApnPushStrip } from "@/components/strip/ApnPushStrip";
import { ApnTaxiDepStrip } from "@/components/strip/ApnTaxiDepStrip";
import { ClxClearedStrip } from "@/components/strip/ClxClearedStrip";
import { DelStrip } from "@/components/strip/DelStrip";
import { FinalArrStrip } from "@/components/strip/FinalArrStrip";
import { GroundStrip } from "@/components/strip/GroundStrip";
import { TwyDepStrip } from "@/components/strip/TwyDepStrip";
import { HalfStrip } from "@/components/strip/HalfStrip";
import { Bay, CommunicationType, type FrontendStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore } from "@/store/store";
import { WebSocketStoreContext } from "@/store/store-context";
import "@/index.css";

const params = new URLSearchParams(location.search);
const scenario = params.get("scenario");
const closed = scenario === "closed" || scenario === "selected" || scenario === "inactive" || scenario === "warnings";
const store = createWebSocketStore({ on() {}, send() { return false; }, isConnected() { return false; } } as unknown as WebSocketClient);
store.setState({
  position: "121.730", airport: "EKCH", selectedCallsign: scenario === "selected" ? "SAS123" : null,
  runwaySetup: { ...store.getState().runwaySetup, runway_status: closed ? { "04L-22R": "CLOSED" } : {} },
  stripTransfers: params.get("request") === "none" ? {} : { SAS123: { from: "121.730", to: "118.580", isTagRequest: true } },
  strips: [{
    callsign: "SAS123", origin: "EKCH", destination: "EGLL", alternate: "", route: "", remarks: "",
    runway: "04L", squawk: "", assigned_squawk: "", sid: "", star: "", cleared_altitude: 0,
    requested_altitude: 0, heading: 0, aircraft_type: "B738", aircraft_category: "M", stand: "", capabilities: "",
    communication_type: CommunicationType.Voice, eobt: "", tobt: "", tsat: "", ctot: "", eldt: "",
    bay: "CLX", release_point: "", version: 1, sequence: 1, next_controllers: [], previous_controllers: [],
    owner: "121.730", pdc_state: "NONE", start_req: false, marked: false, runway_cleared: false,
    runway_confirmed: false, registration: "",
    validation_status: {
      issue_type: "STAND ASSIGNMENT", message: "Advisory", owning_position: "121.730",
      active: scenario !== "inactive" && params.get("validation") !== "inactive", activation_key: "contrast-test",
    },
  } satisfies FrontendStrip],
});

createRoot(document.getElementById("root")!).render(
  <WebSocketStoreContext.Provider value={store}>
    <button onClick={() => store.setState({ stripTransfers: {}, selectedCallsign: null })}>Remove request</button>
    {Object.entries({ arrival: ApnArrStrip, pushback: ApnPushStrip, apronTaxi: ApnTaxiDepStrip, cleared: ClxClearedStrip,
      delivery: DelStrip, final: FinalArrStrip, ground: GroundStrip, taxiDep: TwyDepStrip, half: HalfStrip }).map(([name, Strip]) => (
      <div key={name} data-testid={name} style={{ width: "70vw", margin: "4vh auto" }}>
        <Strip callsign="SAS123" runway={closed ? "04L" : undefined} isManual={params.get("manual") !== "false"} selectable bay={Bay.Cleared}
          stand="A18" holdingPoint="K3" unexpectedChangeFields={scenario === "warnings" ? ["runway", "stand", "release_point"] : undefined}
          pdcStatus={scenario === "pdc" ? "CLEARED" : "NONE"} owner="121.730" myPosition="121.730"
          nextControllers={["118.580"]} nextDisplay={{ label: "GW", frequency: "118.580" }} />
      </div>
    ))}
  </WebSocketStoreContext.Provider>,
);
