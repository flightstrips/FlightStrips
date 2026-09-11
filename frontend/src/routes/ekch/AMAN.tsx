import {useEffect, useLayoutEffect, useRef, useState} from "react";

import {AMANBoardView} from "@/components/aman/AMANBoard";
import {AMANControls} from "@/components/aman/AMANControls";
import {AMANFlightDetailDialog} from "@/components/aman/AMANFlightDetailDialog";
import {AMANWorkspaceShell} from "@/components/aman/AMANWorkspaceShell";
import {AMANWarningPanel} from "@/components/aman/AMANWarningPanel";
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
  const warnings = useWebSocketStore((value) => value.amanWarnings);
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
    <>
      <AMANWorkspaceShell
        maestro={(
          <AMANBoardView
            connectionState={connectionState}
            error={error}
            onOpenControls={() => controlsRef.current?.focus()}
            onOpenFlightDetails={(flightID) => {
              setSelectedFlightID(flightID);
              setDetailOpen(true);
            }}
            onSelectFlight={setSelectedFlightID}
            presentationStatus={presentationStatus}
            selectedFlightID={effectiveSelectedFlightID}
            state={state}
          />
        )}
        tmt={(
          <>
            <AMANWarningPanel connectionState={connectionState} current={warnings} presentationStatus={presentationStatus} />
            {state?.traffic_prediction !== undefined && <TMTTrafficPrediction prediction={state.traffic_prediction} />}
            {state?.holding_information !== undefined && <TMTHoldingGraph entries={state.holding_information} />}
            <AMANControls
              hasFMPAuthority={hasFMPAuthority}
              onSelectedFlightIDChange={setSelectedFlightID}
              selectedFlightID={effectiveSelectedFlightID}
            />
          </>
        )}
        tmtRef={controlsRef}
      />
      {detailOpen && state !== null && effectiveSelectedFlightID !== null && <AMANFlightDetailDialog airport={state.airport} flightID={effectiveSelectedFlightID} onClose={() => setDetailOpen(false)} />}
    </>
  );
}
