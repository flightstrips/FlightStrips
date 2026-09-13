import type {ReactNode} from "react";

import type {AMANAircraftTargetOptionalField} from "./AMANAircraftTarget";
import {
  AMAN_TARGET_FIELD_LABELS,
  type AMANAircraftTargetPreferences,
  type AMANAircraftTargetSide,
} from "./amanAircraftTargetPreferenceModel";

const FIELD_IDS: AMANAircraftTargetOptionalField[] = ["feeder-fix-eta", "runway", "aircraft-type", "wtc", "feeder-fix", "total-delay"];
const FIGMA_FIELD_LABELS: Record<AMANAircraftTargetOptionalField, string> = {
  "feeder-fix-eta": "STA FF",
  runway: "RWY",
  "aircraft-type": "ATYP",
  wtc: "WTC",
  "feeder-fix": "FF",
  "total-delay": "Total Delay",
};

export interface AMANAircraftTargetPreferenceControlsProps {
  preferences: AMANAircraftTargetPreferences;
  onChange: (preferences: AMANAircraftTargetPreferences) => void;
}

/** Target-information matrix from the MAESTRO design. Current delay is always shown. */
export function AMANAircraftTargetPreferenceControls({preferences, onChange}: AMANAircraftTargetPreferenceControlsProps): ReactNode {
  function toggle(side: AMANAircraftTargetSide, id: AMANAircraftTargetOptionalField) {
    const visible = preferences[side].includes(id);
    onChange({...preferences, [side]: visible ? preferences[side].filter((field) => field !== id) : [...preferences[side], id]});
  }

  return (
    <div className="grid grid-cols-[1fr_4rem_4rem] items-center gap-x-5 gap-y-3 px-6 py-3 font-display text-base font-bold text-white">
      <span aria-hidden="true" />
      <span className="text-center">FF</span>
      <span className="text-center">RWY</span>
      {FIELD_IDS.map((id) => <div className="contents" key={id}>
        <span>{FIGMA_FIELD_LABELS[id]}</span>
        {(["feeder", "runway"] as const).map((side) => {
          const visible = preferences[side].includes(id);
          return <label className="grid place-items-center" key={side}>
            <input
              aria-label={`Show ${AMAN_TARGET_FIELD_LABELS[id]} on ${side} side`}
              checked={visible}
              className="peer sr-only"
              onChange={() => toggle(side, id)}
              type="checkbox"
            />
            <span aria-hidden="true" className="h-7 w-7 border-2 border-[#dcdcdc] bg-[#dcdcdc] peer-checked:bg-[#202020] peer-focus-visible:outline peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-white" />
          </label>;
        })}
      </div>)}
      <span>Current Delay</span>
      <span className="col-span-2 text-center text-sm font-normal text-white/80">Always shown</span>
    </div>
  );
}
