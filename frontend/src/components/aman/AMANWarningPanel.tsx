import {useLayoutEffect, useRef, useState, type KeyboardEvent} from "react";

import type {AMANConnectionState, AMANFlight, AMANPresentationStatus, AMANWarning} from "@/api/aman";
import type {AMANCurrentWarnings} from "@/store/aman-warning-store";

export interface AMANWarningPanelProps {
  current: AMANCurrentWarnings;
  connectionState: AMANConnectionState;
  presentationStatus: AMANPresentationStatus;
  flights?: readonly Pick<AMANFlight, "callsign" | "flight_id">[];
  onNavigateToFlight?: (flightID: string) => boolean;
}

function sourceLabel(source: AMANWarning["source"]): string {
  return source === "technical_health" ? "Technical health" : "Sequence";
}

function warningScope(warning: AMANWarning): string {
  return [
    warning.component && `Component: ${warning.component}`,
    warning.runway_group_id && `Runway group: ${warning.runway_group_id}`,
    warning.flight_id && `Flight: ${warning.flight_id}`,
    warning.related_flight_id && `Related flight: ${warning.related_flight_id}`,
  ].filter(Boolean).join(" · ");
}

function snapshotStatus({current, connectionState, presentationStatus}: AMANWarningPanelProps): string {
  if (connectionState === "disconnected") return "Disconnected — showing the last received warning snapshot.";
  if (presentationStatus === "degraded") return "Stale — showing the latest warning snapshot with degraded AMAN data.";
  if (current.snapshot === "omitted") return "Warning snapshot unavailable from this AMAN publisher.";
  return current.items.length === 0 ? "No current warnings." : `${current.items.length} current warning${current.items.length === 1 ? "" : "s"}.`;
}

export function AMANWarningPanel(props: AMANWarningPanelProps) {
  const {current, flights = [], onNavigateToFlight} = props;
  const panelRef = useRef<HTMLElement>(null);
  const focusedActionRef = useRef<HTMLButtonElement | null>(null);
  const [navigationStatus, setNavigationStatus] = useState("");
  const flightsByID = new Map(flights.map((flight) => [flight.flight_id, flight.callsign]));

  useLayoutEffect(() => {
    if (focusedActionRef.current?.isConnected === false) {
      focusedActionRef.current = null;
      panelRef.current?.focus();
      setNavigationStatus("The focused warning is no longer current. Focus returned to Current warnings.");
    }
  }, [current]);

  const moveActionFocus = (event: KeyboardEvent<HTMLUListElement>) => {
    if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;
    const actions = [...event.currentTarget.querySelectorAll<HTMLButtonElement>("button[data-warning-flight-action]")];
    if (actions.length === 0) return;
    const currentIndex = actions.indexOf(document.activeElement as HTMLButtonElement);
    const nextIndex = event.key === "Home" ? 0
      : event.key === "End" ? actions.length - 1
      : event.key === "ArrowDown" ? (currentIndex + 1 + actions.length) % actions.length
      : (currentIndex - 1 + actions.length) % actions.length;
    event.preventDefault();
    actions[nextIndex].focus();
  };

  const flightAction = (kind: "Primary" | "Related", flightID: string) => {
    const callsign = flightsByID.get(flightID);
    if (callsign === undefined) {
      return <span aria-label={`${kind} flight ${flightID} unavailable`} className="border border-dashed border-slate-500 px-2 py-1 text-slate-300">{kind}: {flightID} · unavailable</span>;
    }
    return (
      <button
        aria-label={`${kind} flight ${callsign}; select in AMAN`}
        className="rounded border border-slate-400 px-2 py-1 text-left text-sky-100 hover:bg-slate-600 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
        data-warning-flight-action=""
        onClick={(event) => {
          const navigated = onNavigateToFlight?.(flightID) ?? false;
          setNavigationStatus(navigated ? `${kind} flight ${callsign} selected.` : `${kind} flight ${callsign} target is unavailable.`);
          if (!navigated) event.currentTarget.focus();
        }}
        onFocus={(event) => { focusedActionRef.current = event.currentTarget; }}
        type="button"
      >
        {kind}: {callsign}
      </button>
    );
  };

  return (
    <section
      aria-labelledby="aman-warning-heading"
      className="shrink-0 border-2 border-slate-500 bg-[#303034] text-slate-100 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
      ref={panelRef}
      tabIndex={0}
    >
      <header className="flex items-center justify-between border-b border-slate-500 px-3 py-2">
        <h2 className="font-display text-sm font-bold uppercase tracking-wide" id="aman-warning-heading">Current warnings</h2>
        <span aria-hidden="true" className="font-mono text-xs">{current.items.length}</span>
      </header>
      <p aria-live="polite" className="px-3 py-2 text-xs text-slate-200" role="status">
        {snapshotStatus(props)}
      </p>
      <p aria-live="polite" className="sr-only">{navigationStatus}</p>
      {current.items.length > 0 && (
        <ul aria-label="Current AMAN warnings" className="divide-y divide-slate-600 border-t border-slate-600" onKeyDown={moveActionFocus}>
          {current.items.map((warning) => (
            <li
              aria-label={`${warning.severity} warning from ${sourceLabel(warning.source)}: ${warning.message}`}
              className={warning.severity === "error" ? "border-l-4 border-red-300 px-3 py-2" : "border-l-4 border-amber-300 px-3 py-2"}
              data-severity={warning.severity}
              data-warning-id={warning.id}
              key={warning.id}
            >
              <div className="mb-1 flex flex-wrap gap-2 font-mono text-[11px] font-bold uppercase">
                <span className={warning.severity === "error" ? "text-red-200" : "text-amber-200"}>
                  <span aria-hidden="true">{warning.severity === "error" ? "!" : "△"} </span>{warning.severity}
                </span>
                <span className="text-slate-300">Source: {sourceLabel(warning.source)}</span>
                <span className="text-slate-400">{warning.code}</span>
              </div>
              <p className="text-sm leading-snug">{warning.message}</p>
              <p className="mt-1 font-mono text-[11px] text-slate-300">{warningScope(warning)}</p>
              {(warning.flight_id || warning.related_flight_id) && <div className="mt-2 flex flex-wrap gap-2 font-mono text-[11px]">
                {warning.flight_id && flightAction("Primary", warning.flight_id)}
                {warning.related_flight_id && flightAction("Related", warning.related_flight_id)}
              </div>}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
