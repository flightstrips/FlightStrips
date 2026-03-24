import { useState, useContext } from "react";
import { produce } from "immer";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogFooter, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useSelectedCallsign } from "@/store/store-hooks";
import { WebSocketStoreContext } from "@/store/store-context";
import { CLS_CMDBTN } from "@/components/strip/shared";
import type { WebSocketState } from "@/store/store";

const toHHMM = (ms: number) => {
  const d = new Date(ms);
  return d.getUTCHours().toString().padStart(2, "0") + d.getUTCMinutes().toString().padStart(2, "0");
};

const isValidHHMM = (v: string) => /^\d{4}$/.test(v) && parseInt(v.slice(0, 2)) < 24 && parseInt(v.slice(2)) < 60;

const TSAT_PRESETS = [
  { label: "ORANGE", bg: "#DD6A12", fg: "white", tsatMs:  7 * 60 * 1000,   tobtMs: -30 * 60 * 1000 },
  { label: "GREEN",  bg: "#16a34a", fg: "white", tsatMs:  0,                tobtMs:  0              },
  { label: "YELLOW", bg: "#F3EA1F", fg: "black", tsatMs: -4.5 * 60 * 1000, tobtMs: null            },
  { label: "RED",    bg: "#dc2626", fg: "white", tsatMs: -7 * 60 * 1000,   tobtMs: -7 * 60 * 1000  },
  { label: "CLR",    bg: "#646464", fg: "white", tsatMs:  0,                tobtMs:  0              },
] as const;

const CTOT_PRESETS = [
  { label: "YELLOW", bg: "#F3EA1F", fg: "black", ctotMs: 10 * 60 * 1000 },
  { label: "BLUE",   bg: "#00008B", fg: "white", ctotMs:  3 * 60 * 1000 },
  { label: "CLR",    bg: "#646464", fg: "white", ctotMs: null            },
] as const;

const CLS_INPUT = "w-16 bg-white text-black font-mono text-center border border-black text-sm px-1 py-0.5";
const CLS_LABEL = "text-xs font-bold w-12 text-right";

export default function CDMSIM() {
  const [open, setOpen] = useState(false);
  const selectedCallsign = useSelectedCallsign();
  const storeCtx = useContext(WebSocketStoreContext);
  const disabled = !selectedCallsign;

  const [manualEobt, setManualEobt] = useState("");
  const [manualTobt, setManualTobt] = useState("");
  const [manualTsat, setManualTsat] = useState("");
  const [manualCtot, setManualCtot] = useState("");

  const handleTsatPreset = (preset: typeof TSAT_PRESETS[number]) => {
    if (!selectedCallsign || !storeCtx) return;
    const updater = preset.label === "CLR"
      ? produce<WebSocketState>((draft) => {
          const idx = draft.strips.findIndex(s => s.callsign === selectedCallsign);
          if (idx !== -1) { draft.strips[idx].tsat = ""; draft.strips[idx].tobt = ""; draft.strips[idx].eobt = ""; }
        })
      : produce<WebSocketState>((draft) => {
          const idx = draft.strips.findIndex(s => s.callsign === selectedCallsign);
          if (idx !== -1) {
            const now = Date.now();
            const tsat = toHHMM(now + preset.tsatMs);
            const tobt = preset.tobtMs !== null ? toHHMM(now + preset.tobtMs) : tsat;
            draft.strips[idx].tsat = tsat;
            draft.strips[idx].tobt = tobt;
            draft.strips[idx].eobt = tobt;
          }
        });
    storeCtx.setState(updater);
    setOpen(false);
  };

  const handleCtotPreset = (preset: typeof CTOT_PRESETS[number]) => {
    if (!selectedCallsign || !storeCtx) return;
    const updater = produce<WebSocketState>((draft) => {
      const idx = draft.strips.findIndex(s => s.callsign === selectedCallsign);
      if (idx !== -1) {
        draft.strips[idx].ctot = preset.ctotMs !== null ? toHHMM(Date.now() + preset.ctotMs) : "";
      }
    });
    storeCtx.setState(updater);
    setOpen(false);
  };

  const handleManualApply = () => {
    if (!selectedCallsign || !storeCtx) return;
    const updater = produce<WebSocketState>((draft) => {
      const idx = draft.strips.findIndex(s => s.callsign === selectedCallsign);
      if (idx === -1) return;
      if (isValidHHMM(manualEobt)) draft.strips[idx].eobt = manualEobt;
      if (isValidHHMM(manualTobt)) draft.strips[idx].tobt = manualTobt;
      if (isValidHHMM(manualTsat)) draft.strips[idx].tsat = manualTsat;
      if (isValidHHMM(manualCtot)) draft.strips[idx].ctot = manualCtot;
    });
    storeCtx.setState(updater);
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={disabled ? undefined : setOpen}>
      <DialogTrigger asChild>
        <button
          disabled={disabled}
          className={`${CLS_CMDBTN} ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
          onClick={() => !disabled && setOpen(true)}
        >
          CDM
        </button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[320px] bg-[#b3b3b3]">
        <VisuallyHidden.Root><DialogTitle>Simulate CDM</DialogTitle></VisuallyHidden.Root>
        <div className="border-2 border-black">
          <div className="p-2 space-y-2">
            <div className="text-xs font-bold text-center">TSAT / TOBT</div>
            <div className="grid grid-cols-2 gap-2">
              {TSAT_PRESETS.map((p) => (
                <Button
                  key={p.label}
                  variant="trf"
                  className="font-bold text-base h-fit py-3"
                  style={{ backgroundColor: p.bg, color: p.fg }}
                  onClick={() => handleTsatPreset(p)}
                >
                  {p.label}
                </Button>
              ))}
            </div>

            <div className="text-xs font-bold text-center">CTOT</div>
            <div className="grid grid-cols-3 gap-2">
              {CTOT_PRESETS.map((p) => (
                <Button
                  key={p.label}
                  variant="trf"
                  className="font-bold text-base h-fit py-3"
                  style={{ backgroundColor: p.bg, color: p.fg }}
                  onClick={() => handleCtotPreset(p)}
                >
                  {p.label}
                </Button>
              ))}
            </div>

            <div className="text-xs font-bold text-center">MANUAL</div>
            <div className="flex flex-col gap-1">
              {([
                ["EOBT", manualEobt, setManualEobt],
                ["TOBT", manualTobt, setManualTobt],
                ["TSAT", manualTsat, setManualTsat],
                ["CTOT", manualCtot, setManualCtot],
              ] as const).map(([label, val, set]) => (
                <div key={label} className="flex items-center gap-2 justify-end">
                  <span className={CLS_LABEL}>{label}</span>
                  <input
                    className={CLS_INPUT}
                    maxLength={4}
                    placeholder="HHMM"
                    value={val}
                    onChange={(e) => set(e.target.value.replace(/\D/g, "").slice(0, 4))}
                  />
                </div>
              ))}
            </div>
            <Button
              variant="trf"
              className="w-full font-bold"
              onClick={handleManualApply}
            >
              SET
            </Button>
          </div>

          <DialogFooter className="flex justify-center w-full h-14">
            <Button variant="darkaction" className="w-4/5" onClick={() => setOpen(false)}>
              ESC
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}
