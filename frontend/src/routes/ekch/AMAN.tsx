import {useEffect, useLayoutEffect, useRef, useState} from "react";

import {AMANBoardView} from "@/components/aman/AMANBoard";
import {AMANControls} from "@/components/aman/AMANControls";
import {AMANCoordinationInbox} from "@/components/aman/AMANCoordinationInbox";
import {AMANFlightDetailDialog} from "@/components/aman/AMANFlightDetailDialog";
import {AMANWorkspaceShell} from "@/components/aman/AMANWorkspaceShell";
import {AMANWarningPanel} from "@/components/aman/AMANWarningPanel";
import {TMTHoldingGraph} from "@/components/aman/TMTHoldingGraph";
import {TMTTrafficPrediction} from "@/components/aman/TMTTrafficPrediction";
import {getAMANMutationBlockReason} from "@/api/aman";
import {markAMANStateReceived, measureAMANStatePaint} from "@/lib/aman-performance";
import {useWebSocketStore} from "@/store/store-hooks";

export default function AMAN() {
  const state = useWebSocketStore((value) => value.amanState);
  const presentationStatus = useWebSocketStore((value) => value.amanPresentationStatus);
  const error = useWebSocketStore((value) => value.amanError);
  const connectionState = useWebSocketStore((value) => value.amanConnectionState);
  const hasFMPAuthority = useWebSocketStore((value) => value.amanFMPAuthority);
  const warnings = useWebSocketStore((value) => value.amanWarnings);
  const readOnly = useWebSocketStore((value) => value.readOnly);
  const pendingCommands = useWebSocketStore((value) => value.amanPendingCommands);
  const commandRejections = useWebSocketStore((value) => value.amanCommandRejections);
  const sendCommand = useWebSocketStore((value) => value.sendAMANCommand);
  const [selectedFlightID, setSelectedFlightID] = useState<string | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [coordinationCommandID, setCoordinationCommandID] = useState<string | null>(null);
  const [missedApproachCommandID, setMissedApproachCommandID] = useState<string | null>(null);
  const [decisionCommandID, setDecisionCommandID] = useState<string | null>(null);
  const [removalCommandID, setRemovalCommandID] = useState<string | null>(null);
  const controlsRef = useRef<HTMLElement>(null);
  const stateAtMount = useRef(state);

  const effectiveSelectedFlightID = state?.flights.some((flight) => flight.flight_id === selectedFlightID)
    ? selectedFlightID
    : state?.flights[0]?.flight_id ?? null;
  const selectedFlight = state?.flights.find((flight) => flight.flight_id === effectiveSelectedFlightID) ?? null;
  const mutationBlockReason = getAMANMutationBlockReason({state, connection_state: connectionState, read_only: readOnly, has_fmp_authority: hasFMPAuthority});

  const navigateToWarningFlight = (flightID: string): boolean => {
    if (!state?.flights.some((flight) => flight.flight_id === flightID)) return false;
    setSelectedFlightID(flightID);
    return true;
  };

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
              setMissedApproachCommandID(null);
              setRemovalCommandID(null);
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
            {!hasFMPAuthority && <AMANCoordinationInbox
              canDecide={getAMANMutationBlockReason({state, connection_state: connectionState, read_only: readOnly, has_fmp_authority: true}) === null}
              deciding={decisionCommandID !== null && pendingCommands[decisionCommandID] !== undefined}
              flights={state?.flights ?? []}
              onDecision={(requestID, decision, reason) => setDecisionCommandID(sendCommand({type: `aman.${decision}_coordination_request`, request_id: requestID, ...(reason ? {reason} : {})}))}
              rejection={decisionCommandID ? commandRejections[decisionCommandID]?.message : null}
              requests={state?.coordination_requests ?? []}
            />}
            <AMANWarningPanel
              connectionState={connectionState}
              current={warnings}
              flights={state?.flights}
              onNavigateToFlight={navigateToWarningFlight}
              presentationStatus={presentationStatus}
            />
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
      {detailOpen && state !== null && effectiveSelectedFlightID !== null && selectedFlight !== null && <AMANFlightDetailDialog airport={state.airport} flightID={effectiveSelectedFlightID} onClose={() => setDetailOpen(false)} missedApproach={{
        blockReason: mutationBlockReason,
        confirmation: selectedFlight.go_around_confirmation,
        confirmed: selectedFlight.lifecycle_state === "go_around",
        pending: missedApproachCommandID !== null && pendingCommands[missedApproachCommandID] !== undefined,
        rejection: missedApproachCommandID ? commandRejections[missedApproachCommandID] ?? null : null,
        onConfirm: () => {
          const detection = selectedFlight.go_around_confirmation;
          setMissedApproachCommandID(sendCommand(detection?.status === "pending"
            ? {type: "aman.confirm_go_around", flight_id: effectiveSelectedFlightID, episode_id: detection.episode_id}
            : {type: "aman.report_go_around", flight_id: effectiveSelectedFlightID, detected_at: new Date().toISOString()}));
        },
      }} removal={{
        blockReason: mutationBlockReason,
        confirmed: selectedFlight.lifecycle_state === "removed",
        pending: removalCommandID !== null && pendingCommands[removalCommandID] !== undefined,
        rejection: removalCommandID ? commandRejections[removalCommandID] ?? null : null,
        onConfirm: () => setRemovalCommandID(sendCommand({type: "aman.remove_flight", flight_id: effectiveSelectedFlightID})),
      }} coordination={hasFMPAuthority ? {
        requests: (state.coordination_requests ?? []).filter((request) => request.flight_id === effectiveSelectedFlightID),
        canSubmit: getAMANMutationBlockReason({state, connection_state: connectionState, read_only: readOnly, has_fmp_authority: hasFMPAuthority}) === null,
        submitting: coordinationCommandID !== null && pendingCommands[coordinationCommandID] !== undefined,
        rejection: coordinationCommandID ? commandRejections[coordinationCommandID]?.message : null,
        onSubmit: (submission) => setCoordinationCommandID(sendCommand({type: "aman.submit_coordination_request", flight_id: effectiveSelectedFlightID, ...submission})),
      } : undefined} />}
    </>
  );
}
