import {useEffect, useLayoutEffect, useRef, useState} from "react";

import {AMANBoardView} from "@/components/aman/AMANBoard";
import {AMANControls} from "@/components/aman/AMANControls";
import {markAMANStateReceived, measureAMANStatePaint} from "@/lib/aman-performance";
import {useWebSocketStore} from "@/store/store-hooks";

export default function AMAN() {
  const state = useWebSocketStore((value) => value.amanState);
  const presentationStatus = useWebSocketStore((value) => value.amanPresentationStatus);
  const error = useWebSocketStore((value) => value.amanError);
  const connectionState = useWebSocketStore((value) => value.amanConnectionState);
  const [selectedFlightID, setSelectedFlightID] = useState<string | null>(null);
  const stateAtMount = useRef(state);

  const effectiveSelectedFlightID = state?.flights.some((flight) => flight.flight_id === selectedFlightID)
    ? selectedFlightID
    : state?.flights[0]?.flight_id ?? null;

  useLayoutEffect(() => {
    // A replacement may predate opening the manual AMAN companion view. Reset
    // that initial mark at mount so navigation delay is not counted as paint.
    if (stateAtMount.current !== null) {
      markAMANStateReceived(stateAtMount.current.revision);
    }
  }, []);

  useEffect(() => {
    if (state === null) return undefined;
    return measureAMANStatePaint(state.revision);
  }, [state]);

  return (
    <main className="h-[95.28dvh] overflow-y-auto bg-slate-950 p-3 md:p-5">
      <div className="mx-auto grid max-w-[1920px] items-start gap-4 2xl:grid-cols-[minmax(0,1fr)_420px]">
        <AMANBoardView
          connectionState={connectionState}
          error={error}
          onSelectFlight={setSelectedFlightID}
          presentationStatus={presentationStatus}
          selectedFlightID={effectiveSelectedFlightID}
          state={state}
        />
        <div className="2xl:sticky 2xl:top-0">
          <AMANControls
            // AMAN_FMP_ROLES is backend-configured and no current wire field
            // exposes this client's capability. Keep controls unauthorized
            // until that server-backed capability is available.
            hasFMPAuthority={false}
            onSelectedFlightIDChange={setSelectedFlightID}
            selectedFlightID={effectiveSelectedFlightID}
          />
        </div>
      </div>
    </main>
  );
}
