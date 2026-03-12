import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { useSelectedCallsign, useWebSocketStore } from "@/store/store-hooks";
import { CLS_BTN } from "@/components/strip/shared";

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_DIALOG_BG  = "bg-[#393939] border-2 border-white p-4 w-80"; // dark panel bg
const CLS_HINT_TEXT  = "text-[#aaa] text-xs mb-1"; // muted hint text
const CLS_INPUT      = "w-full bg-[#555355] text-white border-2 border-white px-2 py-1 text-sm font-bold outline-none mb-3";
const CLS_PRESET_BTN = "w-full bg-[#555355] text-white border-2 border-white px-2 py-1 text-sm font-bold outline-none mb-3 active:bg-[#424242]";
const CLS_CANCEL_BTN = `${CLS_BTN} py-1`; // standard btn + py-1

interface Props {
  open: boolean;
  bay: string;
  onOpenChange: (open: boolean) => void;
}

const configuredLabels: string[] = ["SEPARATION BETWEEN STARTS 3 MIN" ,"STOP CLIMB AT 3000'", "STOP CLIMB AT 4000'"]

export function MemaidDialog({ open, bay, onOpenChange }: Props) {
  const [label, setLabel] = useState("");

  const createTacticalStrip = useWebSocketStore(s => s.createTacticalStrip);
  const selectedAircraft = useSelectedCallsign();

  function handleSubmit(configuredLabel?: string) {
    if (!label.trim() && !configuredLabel?.trim()) return;
    createTacticalStrip("MEMAID", bay, configuredLabel?.trim() ?? label.trim(), selectedAircraft ?? '');
    setLabel("");
    onOpenChange(false);
  }

  function handleCancel() {
    setLabel("");
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG_BG}>
        <DialogTitle className="text-white font-bold text-lg mb-2">NEW MEMAID</DialogTitle>
        {selectedAircraft && (
          <p className={CLS_HINT_TEXT}>Aircraft: {selectedAircraft}</p>
        )}
        <input
          autoFocus
          value={label}
          onChange={e => setLabel(e.target.value)}
          onKeyDown={e => {
            if (e.key === "Enter") handleSubmit();
            if (e.key === "Escape") handleCancel();
          }}
          placeholder="Memory aid message…"
          className={CLS_INPUT}
        />
        { configuredLabels.map(l =>
          <div>
            <button className={CLS_PRESET_BTN} onClick={() => handleSubmit(l)}>{l}</button>
          </div>
        ) }
        <div className="flex gap-2 justify-end">
          <button
            className={CLS_CANCEL_BTN}
            onClick={handleCancel}
          >
            CANCEL
          </button>
          <button
            className="bg-primary text-white font-bold text-sm px-4 py-1 border-2 border-white active:bg-primary/80 disabled:opacity-40"
            onClick={() => handleSubmit()}
            disabled={!label.trim()}
          >
            OK
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
