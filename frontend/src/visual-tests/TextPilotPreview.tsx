import { ApnPushStrip } from "@/components/strip/ApnPushStrip";
import { ApnTaxiDepStrip } from "@/components/strip/ApnTaxiDepStrip";
import { TwyDepStrip } from "@/components/strip/TwyDepStrip";
import { FinalArrStrip } from "@/components/strip/FinalArrStrip";
import { ApnArrStrip } from "@/components/strip/ApnArrStrip";
import { DelStrip } from "@/components/strip/DelStrip";
import { ClxClearedStrip } from "@/components/strip/ClxClearedStrip";
import { TextPilotIndicator } from "@/components/strip/TextPilotIndicator";
import { useState } from "react";
import { PushbackMapDialog } from "@/components/map-dialogs/PushbackMapDialog";
import { createRoot } from "react-dom/client";
import "@/index.css";
import { TextPilotDialog } from "@/components/text-pilot/TextPilotDialog";
import { WebSocketStoreContext } from "@/store/store-context";
import { createWebSocketStore } from "@/store/store";
import type { WebSocketClient } from "@/api/websocket";
import { CommunicationType, type FrontendStrip } from "@/api/models";
import type { TextPilotVariant } from "@/lib/text-pilot";

const variants: TextPilotVariant[] = ["push", "apron-out", "apron-in", "tower-out", "tower-in", "si", "runway"];
const requested = new URLSearchParams(location.search).get("variant") as TextPilotVariant;
const variant = variants.includes(requested) ? requested : "runway";
const indicatorEntry = new URLSearchParams(location.search).get("entry") === "indicators";
const mapEntry = new URLSearchParams(location.search).get("entry") === "push-map";
const store = createWebSocketStore({ on() {}, send() { return false; }, isConnected() { return false; } } as unknown as WebSocketClient);
store.setState({
  readOnly: false, position: "121.905", metar: "EKCH 101220Z 24012KT CAVOK 12/10 Q1013",
  strips: [{ callsign: "SAS123", runway: "22R", owner: "121.905", communication_type: CommunicationType.Text }] as FrontendStrip[],
  textPilotDraft: mapEntry || indicatorEntry ? null : { callsign: "SAS123", variant, point: variant === "push" ? "J4" : variant === "tower-in" ? "K3" : "A/B", splitTowerDefaults: true, stand: variant === "apron-in" ? "A18" : undefined, confirmed: true, initialSelected: variant === "si" ? ["readback", "freq:121.730"] : undefined },
});
function Preview() {
  const [mapOpen, setMapOpen] = useState(mapEntry);
  return <WebSocketStoreContext.Provider value={store}>
    {indicatorEntry && <div style={{ width: "70vw", margin: "10vh auto" }}>
      {[CommunicationType.Text, CommunicationType.Receive, CommunicationType.Voice].map(type => <div key={type} style={{ marginBottom: 24 }}>
        {[DelStrip, ClxClearedStrip, ApnPushStrip, ApnTaxiDepStrip, TwyDepStrip].map((Strip, index) => <div key={index} data-testid="indicator-departure">
          <Strip callsign="SAS123" destination="EGLL" stand="A18" runway="22R" aircraftType="B738" communicationType={type} />
        </div>)}
        {[FinalArrStrip, ApnArrStrip].map((Strip, index) => <div key={index} data-testid="indicator-arrival">
          <Strip callsign="SAS123" destination="EKCH" stand="A18" runway="22L" aircraftType="B738" communicationType={type} arrival />
        </div>)}
      </div>)}
      {[18, 30, 51].map(height => <div key={height} data-testid="indicator-cell" style={{ position: "relative", height, width: 160, marginTop: 20 }}>
        <TextPilotIndicator type={CommunicationType.Text} />
      </div>)}
    </div>}
    {mapEntry && <PushbackMapDialog open={mapOpen} onOpenChange={setMapOpen} callsign="SAS123" />}
    <TextPilotDialog />
  </WebSocketStoreContext.Provider>;
}
createRoot(document.getElementById("root")!).render(<Preview />);
