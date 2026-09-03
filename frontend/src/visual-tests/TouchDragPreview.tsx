import { createRoot } from "react-dom/client";
import { useState } from "react";

import "@/index.css";
import { Bay, type StripRef, type TacticalStrip } from "@/api/models";
import type { WebSocketClient } from "@/api/websocket";
import { SortableBay } from "@/components/bays/SortableBay";
import { ViewDndContext } from "@/components/bays/ViewDndContext";
import { TwyDepStrip } from "@/components/strip/TwyDepStrip";
import { WebSocketStoreContext } from "@/store/store-context";
import { createWebSocketStore } from "@/store/store";

/* eslint-disable react-refresh/only-export-components -- isolated Playwright fixture entry point */

const sourceStrip: TacticalStrip = {
  id: 1,
  session_id: 1,
  type: "MEMAID",
  bay: Bay.Taxi,
  label: "SAS123",
  aircraft: "A320",
  produced_by: "",
  owner: "",
  marked: false,
  sequence: 1,
  confirmed: true,
  confirmed_by: "",
  created_at: "2026-09-03T00:00:00Z",
};

const scrollStrips: TacticalStrip[] = Array.from({ length: 8 }, (_, index) => ({
  ...sourceStrip,
  id: index + 10,
  label: `SCROLL ${index + 1}`,
  sequence: index + 1,
}));

const store = createWebSocketStore({ send: () => {}, on: () => {} } as unknown as WebSocketClient);

function StripFace({ testId, label }: { testId: string; label: string }) {
  return (
    <div data-testid={testId} className="h-16 border-2 border-cyan-500 bg-cyan-100 p-4 font-bold">
      {label}
    </div>
  );
}

function SourceStripFace() {
  return (
    <div data-testid="source-strip" className="w-full">
      <TwyDepStrip
        callsign="SAS123"
        bay={Bay.TaxiLwr}
        aircraftType="A320"
        squawk="1234"
        stand="A20"
        runway="04R"
        holdingPoint="B3"
        clearedAltitude={70}
        sid="SIMEG2A"
        destination="ESSA"
        owner=""
        myPosition=""
      />
    </div>
  );
}

function TouchDragPreview() {
  const [sourceStrips, setSourceStrips] = useState<TacticalStrip[]>([sourceStrip]);
  const [targetStrips, setTargetStrips] = useState<TacticalStrip[]>([]);
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const bayStripMap = {
    SOURCE: { strips: sourceStrips, targetBay: Bay.Taxi },
    TARGET: { strips: targetStrips, targetBay: Bay.Depart },
    SCROLL: { strips: scrollStrips, targetBay: Bay.TaxiLwr },
  };

  function handleMove(strip: StripRef, bay: Bay) {
    setDropTarget(bay);
    if (strip.kind === "tactical" && strip.id === sourceStrip.id && bay === Bay.Depart) {
      setSourceStrips([]);
      setTargetStrips([sourceStrip]);
    }
  }

  return (
    <WebSocketStoreContext.Provider value={store}>
      <ViewDndContext
        bayStripMap={bayStripMap}
        transferRules={{ SOURCE: ["TARGET"], TARGET: ["SOURCE"], SCROLL: ["TARGET"] }}
        onReorder={() => {}}
        onMove={handleMove}
        renderDragOverlay={(strip) => (
          <div data-testid="drag-overlay" className="w-72">
            <StripFace testId="drag-overlay-face" label={"label" in strip ? strip.label : strip.callsign} />
          </div>
        )}
      >
        <main className="flex min-h-screen gap-8 bg-neutral-700 p-8">
          <SortableBay strips={sourceStrips} bayId="SOURCE" standalone={false} className="h-80 w-80 bay-scroll-area">
            {() => <SourceStripFace />}
          </SortableBay>
          <SortableBay strips={targetStrips} bayId="TARGET" standalone={false} className="h-80 w-80 bay-scroll-area">
            {(strip) => <StripFace testId="target-strip" label={"label" in strip ? strip.label : strip.callsign} />}
          </SortableBay>
          <div data-testid="scroll-bay" className="h-40 w-80">
            <SortableBay strips={scrollStrips} bayId="SCROLL" standalone={false} className="h-full w-full bay-scroll-area">
              {(strip) => <StripFace testId={`scroll-strip-${"id" in strip ? strip.id : strip.callsign}`} label={"label" in strip ? strip.label : strip.callsign} />}
            </SortableBay>
          </div>
        </main>
      </ViewDndContext>
      <output data-testid="drag-status">{JSON.stringify({ dropTarget })}</output>
    </WebSocketStoreContext.Provider>
  );
}

createRoot(document.getElementById("root")!).render(<TouchDragPreview />);
