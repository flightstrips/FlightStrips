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
  flight: Pick<AMANFlight, "callsign" | "data_status" | "freeze_reason" | "gain_loss_seconds" | "lifecycle_state">;
  guidance: Omit<AMANGainLossPresentationContext, "fresh">;
  selected?: boolean;
  disabled?: boolean;
  leadingFields?: readonly AMANAircraftTargetField[];
  trailingFields?: readonly AMANAircraftTargetField[];
  onSelect?: () => void;
}

function lifecyclePresentation(flight: AMANAircraftTargetProps["flight"]): {label: string; shortLabel: string; tone: string} {
  const freeze = freezePresentation(flight.freeze_reason);
  if (freeze && flight.freeze_reason !== "manual") return freeze;

  switch (flight.lifecycle_state) {
    case "unstable":
      return {label: "Unstable", shortLabel: "U", tone: "bg-[#6e996e] text-white"};
    case "stable":
      if (freeze) return freeze;
      return {
        label: "Stable",
        shortLabel: "S",
        tone: "bg-[#96d796] text-[#202020]",
      };
    default:
      return {
        label: flight.lifecycle_state.replace("_", " "),
        shortLabel: flight.lifecycle_state === "go_around" ? "GA" : flight.lifecycle_state.slice(0, 1).toUpperCase(),
        tone: "bg-[#707070] text-white",
      };
  }
}

function delayTone(delay: string): string {
  if (delay === "Unavailable") return "bg-[#555355] text-white";
  if (!delay.startsWith("L")) return "bg-[#96d796] text-[#202020]";
  return Number.parseInt(delay.slice(1), 10) >= 4
    ? "bg-[#9c0000] text-white"
    : "bg-[#f0e129] text-[#202020]";
}

function TargetFields({fields}: {fields: readonly AMANAircraftTargetField[]}): ReactNode {
  return fields.map((field) => (
    <span className="truncate border-r border-white/40 px-1.5" data-field={field.id} key={field.id} title={field.label}>
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
}: AMANAircraftTargetProps) {
  const delay = formatGainLoss(flight.gain_loss_seconds, {
    ...guidance,
    fresh: flight.data_status === "fresh",
  });
  const lifecycle = lifecyclePresentation(flight);

  return (
    <button
      aria-label={`Select ${flight.callsign}; ${lifecycle.label}; current delay ${delay}`}
      aria-pressed={selected}
      className={cn(
        "group inline-flex min-h-7 max-w-[358px] items-stretch overflow-hidden rounded-sm border border-[#b8b8b8] bg-[#3f3f3f] text-left font-mono text-[11px] font-semibold leading-none text-white shadow-[0_1px_2px_rgb(0_0_0_/_70%)]",
        "hover:rounded hover:border-white hover:bg-[#a3d5e8] hover:text-white focus-visible:rounded focus-visible:border-white focus-visible:bg-[#a3d5e8] focus-visible:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white",
        "disabled:cursor-not-allowed disabled:opacity-60",
        selected && "ring-2 ring-[#f3d02e] ring-offset-1 ring-offset-[#555355]",
      )}
      disabled={disabled}
      onClick={onSelect}
      type="button"
    >
      <TargetFields fields={leadingFields} />
      <span className="flex min-w-20 items-center truncate px-2">{flight.callsign}</span>
      <span className={cn("flex min-w-10 items-center justify-center border-l border-white/40 px-1.5", delayTone(delay))}>{delay}</span>
      <TargetFields fields={trailingFields} />
      <span className={cn("flex min-w-7 items-center justify-center border-l border-white/40 px-1", lifecycle.tone)} title={lifecycle.label}>
        {lifecycle.shortLabel}
      </span>
      {selected && <span className="sr-only">Selected</span>}
    </button>
  );
}
