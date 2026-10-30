import {createRoot} from "react-dom/client";

import type {AMANStateEvent} from "@/api/aman";
import type {WebSocketClient} from "@/api/websocket";
import AMAN from "@/routes/ekch/AMAN";
import {createWebSocketStore} from "@/store/store";
import {WebSocketStoreContext} from "@/store/store-context";
import goldenJSON from "../../../backend/pkg/events/frontend/testdata/aman-state-v1.json?raw";
import "@/index.css";

const state = (JSON.parse(goldenJSON) as AMANStateEvent).data;
const params = new URLSearchParams(location.search);
const interval = params.get("gap") === "clipped"
  ? ["11:28:00", "11:32:00"]
  : params.get("gap") === "short" ? ["10:18:00", "10:18:06"] : ["10:18:00", "10:22:00"];
state.runway_groups[0].gaps = [{
  id: "review-gap", start: `2026-07-22T${interval[0]}.000Z`, end: `2026-07-22T${interval[1]}.000Z`,
  label: "Inserted spacing", created_at: state.generated_at, created_by: "FMP",
}];
state.timeline_configuration = {version: "review-v1", mappings: [
  {id: 1, left: "LUGAS", right: "ROSBI"},
  {id: 2, left: null, right: "ERNOV"},
  {id: 3, left: "OLPIB", right: "TIDVU"},
]};
state.flights = ["LUGAS", "ROSBI", "ERNOV", "OLPIB", "TIDVU"].map((family, index) => ({
  ...state.flights[0], callsign: `SAS${123 + index}`, star_family: family,
  approach_release_time: `2026-07-22T10:${String(17 - index * 3).padStart(2, "0")}:00.000Z`,
  slot: {...state.flights[0].slot!, time: `2026-07-22T10:${String(17 - index * 3).padStart(2, "0")}:00.000Z`},
}));
const store = createWebSocketStore({
  on: () => undefined,
  send: (message: unknown) => {
    document.getElementById("preview-commands")!.textContent = JSON.stringify(message);
    return true;
  },
} as unknown as WebSocketClient);
store.setState({
  airport: "EKCH", amanSessionID: 1, amanConnectionState: "connected", amanState: state,
  amanPresentationStatus: "ready", amanFMPAuthority: true, readOnly: params.has("readOnly"),
});

createRoot(document.getElementById("root")!).render(
  <WebSocketStoreContext.Provider value={store}><AMAN /></WebSocketStoreContext.Provider>,
);
