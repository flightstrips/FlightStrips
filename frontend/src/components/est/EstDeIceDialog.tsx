import * as VisuallyHidden from "@radix-ui/react-visually-hidden";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import type { FrontendStrip } from "@/api/models";
import { scalePx } from "@/lib/viewportScale";
import { EST_RAISED_BUTTON_EDGE, EST_RAISED_EDGE, EST_SUNKEN_EDGE } from "@/components/est/bevel";
import { DEICE_INDICATION_BG, DEICE_INDICATION_FG, formatDeIceIndication, normalizeDeIcePlatform } from "@/components/est/deiceIndication";

interface EstDeIceDialogProps {
  open: boolean;
  strip: FrontendStrip | undefined;
  selectedPlatform?: string;
  onOpenChange: (open: boolean) => void;
  onSelectPlatform: (platform: string) => void;
  onErase: () => void;
}

const PLATFORMS: { label: string; value: string }[] = [
  { label: "PLATFORM A", value: "A" },
  { label: "PLATFORM B", value: "B" },
  { label: "PLATFORM V", value: "V" },
];

const PLATFORM_OPERATORS: Record<string, string> = {
  A: "SAS",
  B: "AVIATOR",
  V: "MENZIES",
};

const PLATFORM_NAMES: Record<string, string> = {
  A: "ALPHA",
  B: "BRAVO",
  V: "VICTOR",
};

const AIRCRAFT_TYPE_RULES: Record<string, string> = {
  A388: "A",
  B748: "A",
};

const AIRLINE_RULES: [ReadonlySet<string>, string][] = [
  [new Set(["SAS", "SZS", "BRX", "BCY", "BLX", "SIN", "GRL"]), "A"],
  [
    new Set([
      "AEE", "BTI", "AFR", "MSC", "GRL", "ASL", "PNX", "AUA", "BEL", "CTN",
      "CAI", "ETH", "ICE", "KLM", "LOT", "DLH", "LGL", "SWR", "TRF", "TRA",
      "WZZ", "WLM", "RUK", "RYR", "LDM", "VOE", "IAW", "MMD", "EZY", "EZS",
      "EJU", "JTD", "JTG", "CCA", "PGT", "WUK", "WAZ", "WMT", "RYS", "MAY",
    ]),
    "B",
  ],
  [
    new Set([
      "ACA", "AAL", "BGH", "BAW", "DAL", "EWG", "FIN", "IBS", "EXS", "SXS",
      "TAP", "THY", "VLG", "UAE", "DTR", "DNU", "QTR", "MSR", "THA", "AIC",
    ]),
    "V",
  ],
];

const STAND_RULES: Record<string, string> = {
  B: "A",
  E: "A",
  A: "B",
  F: "B",
};

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_DIALOG = "rounded-none border border-black bg-[#B3B3B3] text-black";
const CLS_READOUT = "flex items-center justify-center bg-[#d6d6d6] font-bold";

function normalizePlatform(platform: string | undefined) {
  return normalizeDeIcePlatform(platform);
}


function resolveSemPlatform(strip: FrontendStrip | undefined) {
  const aircraftType = strip?.aircraft_type.trim().toUpperCase() ?? "";
  const aircraftPlatform = AIRCRAFT_TYPE_RULES[aircraftType];
  if (aircraftPlatform) {
    return aircraftPlatform;
  }

  const airline = strip?.callsign.trim().slice(0, 3).toUpperCase() ?? "";
  for (const [airlines, platform] of AIRLINE_RULES) {
    if (airlines.has(airline)) {
      return platform;
    }
  }

  const stand = strip?.stand_assignment?.stand || strip?.stand || "";
  const standPlatform = STAND_RULES[stand.trim().charAt(0).toUpperCase()];
  return standPlatform || "A";
}

export default function EstDeIceDialog({
  open,
  strip,
  selectedPlatform,
  onOpenChange,
  onSelectPlatform,
  onErase,
}: EstDeIceDialogProps) {
  const assigned = normalizePlatform(selectedPlatform);
  const sem = resolveSemPlatform(strip);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG} style={{ width: scalePx(270), padding: scalePx(12), ...EST_RAISED_EDGE }}>
        <VisuallyHidden.Root>
          <DialogTitle>De-ice {strip?.callsign ?? ""}</DialogTitle>
        </VisuallyHidden.Root>

        <div className="flex items-end" style={{ gap: scalePx(6) }}>
          <div className="flex-1">
            <div className="text-center font-bold" style={{ fontSize: scalePx(11), marginBottom: scalePx(5) }}>
              DEICE OPERATOR
            </div>
            <div className={CLS_READOUT} style={{ height: scalePx(40), fontSize: scalePx(17), ...EST_SUNKEN_EDGE }}>
              {PLATFORM_OPERATORS[sem]}
            </div>
          </div>
          <div style={{ width: scalePx(90) }}>
            <div className="text-center font-bold" style={{ fontSize: scalePx(11), marginBottom: scalePx(5) }}>
              SEM PLAT
            </div>
            <div className={CLS_READOUT} style={{ height: scalePx(40), fontSize: scalePx(17), ...EST_SUNKEN_EDGE }}>
              {PLATFORM_NAMES[sem]}
            </div>
          </div>
        </div>

        <fieldset
          className="border border-black"
          style={{ marginTop: scalePx(10), padding: `0 ${scalePx(9)} ${scalePx(9)}` }}
        >
          <legend
            className="mx-auto font-bold"
            style={{ fontSize: scalePx(11), padding: `0 ${scalePx(6)}` }}
          >
            ASSIGNED PLATFORM
          </legend>

          <div className="text-center font-bold" style={{ fontSize: scalePx(11), marginBottom: scalePx(6) }}>
            DE-ICE INDICATION
          </div>
          <div
            className="flex items-center justify-center bg-white font-bold"
            style={{
              height: scalePx(40),
              fontSize: scalePx(17),
              ...EST_SUNKEN_EDGE,
              ...(assigned ? { backgroundColor: DEICE_INDICATION_BG, color: DEICE_INDICATION_FG } : {}),
            }}
          >
            {assigned ? formatDeIceIndication(assigned) : "\u00a0"}
          </div>

          <div className="grid" style={{ marginTop: scalePx(10), gap: scalePx(6) }}>
            {PLATFORMS.map((platform) => (
              <Button
                key={platform.value}
                variant="trf"
                className="font-bold"
                style={{ height: scalePx(40), fontSize: scalePx(15), ...EST_RAISED_BUTTON_EDGE }}
                onClick={() => onSelectPlatform(platform.value)}
              >
                {platform.label}
              </Button>
            ))}
            <Button
              variant="trf"
              className="font-bold"
              style={{ height: scalePx(40), fontSize: scalePx(15), ...EST_RAISED_BUTTON_EDGE }}
              onClick={onErase}
            >
              NO DE-ICE
            </Button>
          </div>

          <div className="flex justify-center" style={{ marginTop: scalePx(10) }}>
            <Button
              variant="darkaction"
              style={{ width: scalePx(120), height: scalePx(40), fontSize: scalePx(19), ...EST_RAISED_BUTTON_EDGE }}
              onClick={() => onOpenChange(false)}
            >
              OK
            </Button>
          </div>
        </fieldset>
      </DialogContent>
    </Dialog>
  );
}
