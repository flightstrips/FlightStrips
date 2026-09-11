import {useEffect, useLayoutEffect, useRef, useState} from "react";

import {AMANBoardView} from "@/components/aman/AMANBoard";
import {AMANControls} from "@/components/aman/AMANControls";
import {AMANFlightDetailDialog} from "@/components/aman/AMANFlightDetailDialog";
import {TMTHoldingGraph} from "@/components/aman/TMTHoldingGraph";
import {TMTTrafficPrediction} from "@/components/aman/TMTTrafficPrediction";
import {markAMANStateReceived, measureAMANStatePaint} from "@/lib/aman-performance";
import {useWebSocketStore} from "@/store/store-hooks";

export default function AMAN() {
  const state = useWebSocketStore((value) => value.amanState);
  const presentationStatus = useWebSocketStore((value) => value.amanPresentationStatus);
  const error = useWebSocketStore((value) => value.amanError);
  const connectionState = useWebSocketStore((value) => value.amanConnectionState);
  const hasFMPAuthority = useWebSocketStore((value) => value.amanFMPAuthority);
  const [selectedFlightID, setSelectedFlightID] = useState<string | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const controlsRef = useRef<HTMLElement>(null);
  const stateAtMount = useRef(state);

  const effectiveSelectedFlightID = state?.flights.some((flight) => flight.flight_id === selectedFlightID)
    ? selectedFlightID
    : state?.flights[0]?.flight_id ?? null;

  useLayoutEffect(() => {
    if (stateAtMount.current !== null) markAMANStateReceived(stateAtMount.current.revision);
  }, []);

  useEffect(() => {
    if (state === null) return undefined;
    return measureAMANStatePaint(state.revision);
  }, [state]);

  return (
    <main className="h-[95.28dvh] overflow-hidden bg-[#242424] p-3 text-white">
      <div className="grid h-full min-h-0 grid-cols-[minmax(0,1fr)_minmax(320px,420px)] items-start gap-3">
      <AMANBoardView
        connectionState={connectionState}
        error={error}
        onOpenControls={() => controlsRef.current?.focus()}
        onOpenFlightDetails={() => setDetailOpen(true)}
        onSelectFlight={setSelectedFlightID}
        presentationStatus={presentationStatus}
        selectedFlightID={effectiveSelectedFlightID}
        state={state}
      />
      <aside className="flex h-full min-h-0 flex-col gap-3 overflow-y-auto pr-1" ref={controlsRef} tabIndex={-1}>
        {state?.traffic_prediction !== undefined && <TMTTrafficPrediction prediction={state.traffic_prediction} />}
        {state?.holding_information !== undefined && <TMTHoldingGraph entries={state.holding_information} />}
        <AMANControls
          hasFMPAuthority={hasFMPAuthority}
          onSelectedFlightIDChange={setSelectedFlightID}
          selectedFlightID={effectiveSelectedFlightID}
        />
      </aside>
      </div>
      {detailOpen && state !== null && effectiveSelectedFlightID !== null && <AMANFlightDetailDialog airport={state.airport} flightID={effectiveSelectedFlightID} onClose={() => setDetailOpen(false)} />}
    </main>
  );
}
