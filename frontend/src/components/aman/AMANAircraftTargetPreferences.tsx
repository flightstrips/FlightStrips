import type {ReactNode} from "react";

import {Button} from "@/components/ui/button";
import type {AMANAircraftTargetOptionalField} from "./AMANAircraftTarget";
import {
  AMAN_TARGET_FIELD_LABELS,
  type AMANAircraftTargetPreferences,
  type AMANAircraftTargetSide,
} from "./amanAircraftTargetPreferenceModel";

const FIELD_IDS = Object.keys(AMAN_TARGET_FIELD_LABELS) as AMANAircraftTargetOptionalField[];

function move(order: AMANAircraftTargetOptionalField[], id: AMANAircraftTargetOptionalField, offset: -1 | 1) {
  const from = order.indexOf(id);
  const to = from + offset;
  if (from < 0 || to < 0 || to >= order.length) return order;
  const next = [...order];
  [next[from], next[to]] = [next[to], next[from]];
  return next;
}

export interface AMANAircraftTargetPreferenceControlsProps {
  preferences: AMANAircraftTargetPreferences;
  onChange: (preferences: AMANAircraftTargetPreferences) => void;
}

/** Reusable local settings content; its eventual container remains caller-owned. */
export function AMANAircraftTargetPreferenceControls({preferences, onChange}: AMANAircraftTargetPreferenceControlsProps): ReactNode {
  function setOrder(side: AMANAircraftTargetSide, order: AMANAircraftTargetOptionalField[]) {
    onChange({...preferences, [side]: order});
  }

  return (
    <div className="grid gap-3 text-sm text-foreground sm:grid-cols-2">
      {(["feeder", "runway"] as const).map((side) => (
        <fieldset className="rounded-sm border border-border p-2" key={side}>
          <legend className="px-1 font-semibold">{side === "feeder" ? "Feeder-fix-side" : "Runway-side"} target fields</legend>
          <div className="mt-1 grid gap-1">
            {FIELD_IDS.map((id) => {
              const index = preferences[side].indexOf(id);
              const visible = index >= 0;
              return (
                <div className="flex min-h-8 items-center gap-2" key={id}>
                  <label className="flex min-w-0 flex-1 items-center gap-2">
                    <input
                      aria-label={`Show ${AMAN_TARGET_FIELD_LABELS[id]} on ${side} side`}
                      checked={visible}
                      onChange={() => setOrder(side, visible ? preferences[side].filter((field) => field !== id) : [...preferences[side], id])}
                      type="checkbox"
                    />
                    <span>{AMAN_TARGET_FIELD_LABELS[id]}</span>
                  </label>
                  <Button
                    aria-label={`Move ${AMAN_TARGET_FIELD_LABELS[id]} earlier on ${side} side`}
                    disabled={!visible || index === 0}
                    onClick={() => setOrder(side, move(preferences[side], id, -1))}
                    size="icon"
                    type="button"
                    variant="ghost"
                  >
                    <span aria-hidden="true">↑</span>
                  </Button>
                  <Button
                    aria-label={`Move ${AMAN_TARGET_FIELD_LABELS[id]} later on ${side} side`}
                    disabled={!visible || index === preferences[side].length - 1}
                    onClick={() => setOrder(side, move(preferences[side], id, 1))}
                    size="icon"
                    type="button"
                    variant="ghost"
                  >
                    <span aria-hidden="true">↓</span>
                  </Button>
                </div>
              );
            })}
          </div>
        </fieldset>
      ))}
    </div>
  );
}
