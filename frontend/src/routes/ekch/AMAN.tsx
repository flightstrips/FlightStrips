import {useEffect, useLayoutEffect, useRef, useState} from "react";

import {AMANBoardView} from "@/components/aman/AMANBoard";
import {AMANControls} from "@/components/aman/AMANControls";
import {AMANCoordinationInbox} from "@/components/aman/AMANCoordinationInbox";
import {AMANCoordinationRequestDialog} from "@/components/aman/AMANCoordinationRequestDialog";
import {AMANFlightDetailDialog} from "@/components/aman/AMANFlightDetailDialog";
import {AMANWorkspaceShell} from "@/components/aman/AMANWorkspaceShell";
import {AMANWarningPanel} from "@/components/aman/AMANWarningPanel";
import {TMTHoldingGraph} from "@/components/aman/TMTHoldingGraph";
import {TMTTrafficPrediction} from "@/components/aman/TMTTrafficPrediction";
import {getAMANMutationBlockReason} from "@/api/aman";
import {markAMANStateReceived, measureAMANStatePaint} from "@/lib/aman-performance";
import {useWebSocketStore} from "@/store/store-hooks";
import {orderedEKCHTMTHoldings} from "@/config/aman";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";

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
  const [detailAction, setDetailAction] = useState<"none" | "all" | "missed" | "remove">("none");
  const [flightActionsOpen, setFlightActionsOpen] = useState(false);
  const [coordinationOpen, setCoordinationOpen] = useState(false);
  const [coordinationCommandID, setCoordinationCommandID] = useState<string | null>(null);
  const [missedApproachCommandID, setMissedApproachCommandID] = useState<string | null>(null);
  const [decisionCommandID, setDecisionCommandID] = useState<string | null>(null);
  const [removalCommandID, setRemovalCommandID] = useState<string | null>(null);
  const [controlsOpen, setControlsOpen] = useState(false);
  const stateAtMount = useRef(state);

  const effectiveSelectedFlightID = state?.flights.some((flight) => flight.flight_id === selectedFlightID)
    ? selectedFlightID
    : state?.flights[0]?.flight_id ?? null;
  const selectedFlight = state?.flights.find((flight) => flight.flight_id === effectiveSelectedFlightID) ?? null;
  const holdingInformation = state?.holding_information ?? [];
  const tmtHoldings = orderedEKCHTMTHoldings(holdingInformation);
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
            onOpenControls={() => setControlsOpen(true)}
            onOpenFlightActions={(flightID) => {
              setSelectedFlightID(flightID);
              setFlightActionsOpen(true);
            }}
            onOpenFlightDetails={(flightID) => {
              setSelectedFlightID(flightID);
              setDetailAction("all");
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
          <div className="aman-tmt-dashboard">
            <div className="aman-tmt-traffic">
              {state?.traffic_prediction !== undefined
                ? <TMTTrafficPrediction prediction={state.traffic_prediction} />
                : <section className="aman-tmt-placeholder" aria-label="TMT traffic prediction unavailable"><b>TMT · TRAFFIC PREDICTION</b><span>Prediction data unavailable</span></section>}
            </div>
            <div aria-label="TMT holding workspaces" className="aman-tmt-holdings">
              {tmtHoldings.map((holding) => <TMTHoldingGraph compact entries={holdingInformation.filter((entry) => entry.holding === holding)} holding={holding} key={holding} />)}
            </div>
            <div className="aman-tmt-notices">
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
            </div>
          </div>
        )}
      />
      <Dialog onOpenChange={setControlsOpen} open={controlsOpen}>
        <DialogContent className="max-h-[90dvh] w-[min(72rem,calc(100vw-2rem))] max-w-none overflow-y-auto border-slate-600 bg-slate-900 p-0 text-slate-100">
          <DialogTitle className="sr-only">AMAN FMP controls</DialogTitle>
          <AMANControls hasFMPAuthority={hasFMPAuthority} onSelectedFlightIDChange={setSelectedFlightID} selectedFlightID={effectiveSelectedFlightID} />
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={setFlightActionsOpen} open={flightActionsOpen}>
        <DialogContent className="w-40 max-w-[calc(100vw-1rem)] gap-0 rounded-md border-2 border-[#dcdcdc] bg-[#5174b8] p-1 font-display text-[11px] font-bold text-white [&>button]:hidden">
          <DialogTitle className="px-2 py-1 text-center text-xs font-bold text-[#bba8ee]">{selectedFlight?.callsign ?? "Unavailable"}</DialogTitle>
          <div className="grid gap-0">
            <button className="flex min-h-6 items-center rounded-md border border-[#dcdcdc] bg-[#a3d5e8] px-3 text-left text-[#10265c] focus-visible:outline focus-visible:outline-2 focus-visible:outline-white" onClick={() => {
              if (effectiveSelectedFlightID !== null) {
                setFlightActionsOpen(false);
                setDetailAction("none");
                setDetailOpen(true);
              }
            }} type="button">Information</button>
            <button className="min-h-5 px-3 text-left hover:bg-[#6685c2] disabled:opacity-50" disabled={mutationBlockReason !== null || effectiveSelectedFlightID === null} onClick={() => {
              if (effectiveSelectedFlightID !== null) {
                sendCommand({type: "aman.recompute_flight", flight_id: effectiveSelectedFlightID});
                setFlightActionsOpen(false);
              }
            }} type="button">Recompute</button>
            <div className="mx-1 my-0.5 border-t-2 border-white" />
            {[{label: "Alternate Runway", child: false}, {label: "Change Runway", child: true}, {label: "Change ETA-FF", child: false}].map(({label, child}) => (
              <button className="flex min-h-5 items-center justify-between px-3 text-left hover:bg-[#6685c2]" key={label} onClick={() => {
                setFlightActionsOpen(false);
                setControlsOpen(true);
              }} type="button"><span>{label}</span>{child && <span aria-hidden="true">▶</span>}</button>
            ))}
            <div className="mx-1 my-0.5 border-t-2 border-white" />
            <button className="min-h-5 px-3 text-left hover:bg-[#6685c2]" onClick={() => {
              setFlightActionsOpen(false);
              setCoordinationOpen(true);
            }} type="button">Coordination</button>
            <div className="mx-1 my-0.5 border-t-2 border-white" />
            {["Missed Approach", "De-Sequence", "Insert Closure", "Insert Gap"].map((label) => (
              <button className="flex min-h-5 items-center justify-between px-3 text-left hover:bg-[#6685c2]" key={label} onClick={() => {
                setFlightActionsOpen(false);
                if (label === "Missed Approach") {
                  setDetailAction("missed");
                  setDetailOpen(true);
                }
                else setControlsOpen(true);
              }} type="button"><span>{label}</span><span aria-hidden="true">▶</span></button>
            ))}
            <button className="min-h-5 px-3 text-left hover:bg-[#6685c2]" onClick={() => { setFlightActionsOpen(false); setControlsOpen(true); }} type="button">Extra Flight</button>
            <div className="mx-1 my-0.5 border-t-2 border-white" />
            <button className="min-h-5 px-3 text-left hover:bg-[#6685c2]" onClick={() => { setFlightActionsOpen(false); setDetailAction("remove"); setDetailOpen(true); }} type="button">Remove</button>
          </div>
          {mutationBlockReason !== null && <p className="sr-only">Operational changes are currently unavailable: {mutationBlockReason.replaceAll("_", " ")}.</p>}
        </DialogContent>
      </Dialog>
      {coordinationOpen && state !== null && effectiveSelectedFlightID !== null && selectedFlight !== null && <AMANCoordinationRequestDialog
        callsign={selectedFlight.callsign}
        canSubmit={mutationBlockReason === null}
        onClose={() => setCoordinationOpen(false)}
        onSubmit={(submission) => setCoordinationCommandID(sendCommand({type: "aman.submit_coordination_request", flight_id: effectiveSelectedFlightID, ...submission}))}
        rejection={coordinationCommandID ? commandRejections[coordinationCommandID]?.message : null}
        requests={(state.coordination_requests ?? []).filter((request) => request.flight_id === effectiveSelectedFlightID)}
        submitting={coordinationCommandID !== null && pendingCommands[coordinationCommandID] !== undefined}
      />}
      {detailOpen && state !== null && effectiveSelectedFlightID !== null && selectedFlight !== null && <AMANFlightDetailDialog airport={state.airport} flightID={effectiveSelectedFlightID} initialAction={detailAction === "missed" ? "missed-approach" : detailAction === "remove" ? "removal" : undefined} onClose={() => setDetailOpen(false)} missedApproach={detailAction === "none" ? undefined : {
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
      }} removal={detailAction === "none" ? undefined : {
        blockReason: mutationBlockReason,
        confirmed: selectedFlight.lifecycle_state === "removed",
        pending: removalCommandID !== null && pendingCommands[removalCommandID] !== undefined,
        rejection: removalCommandID ? commandRejections[removalCommandID] ?? null : null,
        onConfirm: () => setRemovalCommandID(sendCommand({type: "aman.remove_flight", flight_id: effectiveSelectedFlightID})),
      }} coordination={detailAction === "all" && hasFMPAuthority ? {
        requests: (state.coordination_requests ?? []).filter((request) => request.flight_id === effectiveSelectedFlightID),
        canSubmit: getAMANMutationBlockReason({state, connection_state: connectionState, read_only: readOnly, has_fmp_authority: hasFMPAuthority}) === null,
        submitting: coordinationCommandID !== null && pendingCommands[coordinationCommandID] !== undefined,
        rejection: coordinationCommandID ? commandRejections[coordinationCommandID]?.message : null,
        onSubmit: (submission) => setCoordinationCommandID(sendCommand({type: "aman.submit_coordination_request", flight_id: effectiveSelectedFlightID, ...submission})),
      } : undefined} />}
    </>
  );
}
