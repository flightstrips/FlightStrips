import {createRoot} from "react-dom/client";
import type {WebSocketClient} from "@/api/websocket";
import type {InboundDeparturesSnapshot} from "@/api/inbound-departures";
import {createWebSocketStore} from "@/store/store";
import {WebSocketStoreContext} from "@/store/store-context";
import AMAN from "@/routes/ekch/AMAN";
import "@/index.css";

const flights = Array.from({length: 40}, (_, index) => ({
  callsign: index === 0 ? "FBT08F" : `SAS${index}`, departure: "LFLL", arrival: "EKCH", ctot: "1906", atot: "1924", eta: "2120",
  status: "late" as const, deviation_seconds: 1080, outside_window_seconds: 480,
}));
const snapshot: InboundDeparturesSnapshot = {session_id: 1, airport: "EKCH", enabled: true, updated_at: "2026-10-13T20:43:00Z", stale: false, flights};
const store = createWebSocketStore({on: () => undefined, send: () => undefined} as unknown as WebSocketClient);
store.setState({airport: "EKCH", amanSessionID: 1, amanConnectionState: "connected", inboundDepartures: snapshot});
createRoot(document.getElementById("root")!).render(<WebSocketStoreContext.Provider value={store}><AMAN /></WebSocketStoreContext.Provider>);
