import type {ReactNode} from "react";

import type {AMANFlight} from "@/api/aman";
import {cn} from "@/lib/utils";
import {freezePresentation} from "./freeze-presentation";
import {formatGainLoss, type AMANGainLossPresentationContext} from "./presentation";

export type AMANAircraftTargetOptionalField =
  | "feeder-fix-eta"
  | "total-delay"
  | "runway"
  | "wtc"
  | "aircraft-type"
  | "feeder-fix";

export interface AMANAircraftTargetField {
  id: AMANAircraftTargetOptionalField;
  label: string;
  value: ReactNode;
}

export interface AMANAircraftTargetProps {
  flight: Pick<AMANFlight, "callsign" | "data_status" | "freeze_reason" | "gain_loss_seconds" | "lifecycle_state" | "sequence_disposition" | "runway_gap_exception">;
  guidance: Omit<AMANGainLossPresentationContext, "fresh">;
  selected?: boolean;
  disabled?: boolean;
  leadingFields?: readonly AMANAircraftTargetField[];
  trailingFields?: readonly AMANAircraftTargetField[];
  onSelect?: () => void;
  emphasis?: "primary" | "subdued";
  compact?: boolean;
  delayFirst?: boolean;
}

function lifecyclePresentation(flight: AMANAircraftTargetProps["flight"]): {label: string; tone: string} {
  if (flight.sequence_disposition === "desequenced") {
    return {label: "Desequenced", tone: "text-violet-300"};
  }
  const freeze = freezePresentation(flight.freeze_reason);
  if (freeze && flight.freeze_reason === "superstable") {
    return {label: freeze.label, tone: "text-[#dcdcdc]"};
  }

  switch (flight.lifecycle_state) {
    case "unstable":
      return {label: "Unstable", tone: "text-[#6e996e]"};
    case "stable":
      return {
        label: freeze?.label ?? "Stable",
        tone: "text-[#96d796]",
      };
    default:
      return {
        label: flight.lifecycle_state.replace("_", " "),
        tone: "text-[#dcdcdc]",
      };
  }
}

function delayTone(delay: string): string {
  if (delay === "Unavailable") return "text-[#dcdcdc]";
  if (!delay.startsWith("L")) return "text-[#96d796]";
  return Number.parseInt(delay.slice(1), 10) >= 4
    ? "text-[#e65b5b]"
    : "text-[#f0e129]";
}

function TargetFields({fields}: {fields: readonly AMANAircraftTargetField[]}): ReactNode {
  return fields.map((field) => (
    <span className="flex items-center truncate px-1" data-field={field.id} key={field.id} title={field.label}>
      {field.value}
    </span>
  ));
}

/** Compact MAESTRO target. Field selection and values remain caller-owned. */
export function AMANAircraftTarget({
  flight,
  guidance,
  selected = false,
  disabled = false,
  leadingFields = [],
  trailingFields = [],
  onSelect,
  emphasis,
  compact = false,
  delayFirst = false,
}: AMANAircraftTargetProps) {
  const delay = formatGainLoss(flight.gain_loss_seconds, {
    ...guidance,
    fresh: flight.data_status === "fresh",
  });
  const lifecycle = lifecyclePresentation(flight);

  return (
    <button
      aria-label={`Select ${flight.callsign}; ${lifecycle.label}; current delay ${delay}${emphasis ? `; ${emphasis === "primary" ? "emphasized STAR family" : "other STAR family"}` : ""}`}
      aria-pressed={selected}
      className={cn(
        "group inline-flex min-h-7 max-w-[358px] items-stretch overflow-hidden rounded border border-transparent bg-transparent text-left font-mono text-[11px] font-semibold leading-none text-[#dcdcdc]",
        "hover:border-white hover:bg-[#a3d5e8] hover:text-white [&:hover>span]:!text-white focus-visible:border-white focus-visible:bg-[#a3d5e8] focus-visible:text-white [&:focus-visible>span]:!text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white",
        "disabled:cursor-not-allowed disabled:opacity-60",
        selected && "border-[#f3d02e] bg-[#3f3f3f] ring-2 ring-inset ring-[#f3d02e]",
        emphasis === "primary" && "text-[#96d796] [&>span]:!text-[#96d796]",
        emphasis === "subdued" && "text-[#686868] [&>span]:!text-[#686868]",
      )}
      data-emphasis={emphasis}
      disabled={disabled}
      onClick={onSelect}
      type="button"
    >
      <TargetFields fields={leadingFields} />
      {delayFirst && <span className={cn("flex items-center justify-center", compact ? "min-w-7 px-1" : "min-w-10 px-1.5", delayTone(delay))}>{delay}</span>}
      <span className={cn("flex items-center truncate", compact ? "w-14 min-w-0 px-1.5" : "min-w-20 px-2", lifecycle.tone)}>{flight.callsign}</span>
      {!delayFirst && <span className={cn("flex items-center justify-center", compact ? "min-w-7 px-1" : "min-w-10 px-1.5", delayTone(delay))}>{delay}</span>}
      <TargetFields fields={trailingFields} />
      <span className="sr-only">{lifecycle.label}</span>
      {flight.runway_gap_exception && <span className="flex items-center border-l border-dashed border-amber-200 bg-amber-950 px-1 text-[9px] text-amber-100" title={`Audited manual placement inside GAP ${flight.runway_gap_exception.gap_id}`}>GAP EXCEPTION</span>}
      {selected && <span className="sr-only">Selected</span>}
    </button>
  );
}
