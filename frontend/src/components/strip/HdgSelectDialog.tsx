import { useState, useRef, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";

// Pre-defined headings from HDG.svg (360, 090, 120, 300, 040, 220) in 2-column layout
const HDG_PRESETS = [360, 90, 120, 300, 40, 220];

// Tailwind class constants (hex must be literal strings for JIT) — styled to match HDG.svg
const CLS_DIALOG_BG =
  "bg-[#B3B3B3] border border-black p-0 w-[300px] max-w-none max-h-none gap-0 overflow-hidden [&>button]:hidden";
const CLS_PANEL =
  "mx-[15px] mt-[18px] mb-0 border border-black flex flex-col justify-between";
const CLS_GRID =
  "grid grid-cols-2 gap-x-[32px] gap-y-[15px] p-5 pt-[20px] pb-[12px]";
const CLS_HDG_BTN =
  "w-[99px] h-[54px] bg-[#D6D6D6] text-black font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-95 rounded-none border-0";
const CLS_HDG_BTN_ACTIVE =
  "w-[99px] h-[54px] bg-[#1BFF16] text-black font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-95 rounded-none border-0";
const CLS_CUSTOM_INPUT =
  "w-full h-[54px] bg-[#FDFDFD] border-0 text-black font-semibold text-[28px] text-center shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none rounded-none my-4";
const CLS_BOTTOM_ROW =
  "flex items-center justify-around px-[24px] pb-[24px] pt-[8px]";
const CLS_BOTTOM_BTN =
  "w-[99px] h-[54px] bg-[#3F3F3F] text-white font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90 rounded-none border-0";

function formatHdg(n: number): string {
  return n.toString().padStart(3, "0");
}

/** Returns 0–360 if valid, otherwise undefined. Empty string is invalid. */
function parseHdg(s: string): number | undefined {
  if (s.trim() === "") return undefined;
  const n = parseInt(s, 10);
  if (Number.isNaN(n) || n < 0 || n > 360) return undefined;
  return n;
}

interface HdgSelectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: number | undefined | null;
  onSelect: (heading: number | undefined) => void;
}

export function HdgSelectDialog({
  open,
  onOpenChange,
  value,
  onSelect,
}: HdgSelectDialogProps) {
  const currentHdg = value ?? undefined;
  const [customInput, setCustomInput] = useState("");
  const [customInvalid, setCustomInvalid] = useState(false);
  const [prevOpen, setPrevOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  if (prevOpen !== open) {
    setPrevOpen(open);
    if (open) {
      const presetMatch = HDG_PRESETS.includes(currentHdg ?? -1);
      setCustomInput(
        currentHdg != null && !presetMatch ? formatHdg(currentHdg) : ""
      );
      setCustomInvalid(false);
    }
  }

  // Focus input when dialog opens
  useEffect(() => {
    if (open) {
      const presetMatch = HDG_PRESETS.includes(currentHdg ?? -1);
      if (currentHdg == null || presetMatch) {
        inputRef.current?.focus();
      }
    }
  }, [open, currentHdg]);

  function handleSelect(hdg: number) {
    onSelect(hdg);
    onOpenChange(false);
  }

  function handleErase() {
    onSelect(undefined);
    onOpenChange(false);
  }

  function handleCustomSubmit() {
    const parsed = parseHdg(customInput.trim());
    if (parsed != null) {
      setCustomInvalid(false);
      onSelect(parsed);
      onOpenChange(false);
    } else {
      setCustomInvalid(customInput.trim() !== "");
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG_BG}>
        <DialogTitle className="sr-only">Select heading</DialogTitle>
        <div className={CLS_PANEL}>
          <div className={CLS_GRID}>
            {HDG_PRESETS.map((h) => (
              <button
                key={h}
                type="button"
                className={
                  currentHdg === h ? CLS_HDG_BTN_ACTIVE : CLS_HDG_BTN
                }
                onClick={() => handleSelect(h)}
              >
                {formatHdg(h)}
              </button>
            ))}
          </div>
          <div className="px-5">
            <input
              ref={inputRef}
              type="text"
              inputMode="numeric"
              pattern="[0-9]*"
              maxLength={3}
              placeholder="000–360"
              value={customInput}
              onChange={(e) => {
                const v = e.target.value.replace(/\D/g, "").slice(0, 3);
                setCustomInput(v);
                setCustomInvalid(false);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCustomSubmit();
              }}
              className={CLS_CUSTOM_INPUT}
              style={{
                fontFamily: "Rubik, Arial, sans-serif",
                ...(customInvalid ? { border: "2px solid #b91c1c", boxShadow: "0 0 0 1px #b91c1c" } : {}),
              }}
              aria-invalid={customInvalid}
              aria-describedby={customInvalid ? "hdg-custom-error" : undefined}
            />
            {customInvalid && (
              <p id="hdg-custom-error" className="text-red-700 text-sm mt-1 text-center">
                Enter 000–360
              </p>
            )}
          </div>
          <div className={CLS_BOTTOM_ROW}>
            <button type="button" className={CLS_BOTTOM_BTN} onClick={handleErase}>
              ERASE
            </button>
            <button
              type="button"
              className={CLS_BOTTOM_BTN}
              onClick={() => onOpenChange(false)}
            >
              ESC
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
