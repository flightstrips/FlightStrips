import { useState, useRef, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { useTransitionAltitude } from "@/store/store-hooks";
import { formatAltitude } from "@/lib/utils";

// Pre-defined altitude values in feet (1500, 2500, 3000, 4000, 5000, 7000).
const ALT_PRESET_VALUES = [1500, 2500, 3000, 4000, 5000, 7000];

// Tailwind class constants (hex must be literal strings for JIT) — styled to match ALT.svg
const CLS_DIALOG_BG =
  "bg-[#B3B3B3] border border-black p-0 w-[301px] max-w-none max-h-none gap-0 overflow-hidden [&>button]:hidden";
const CLS_PANEL =
  "mx-[15px] mt-[18px] mb-0 border border-black flex flex-col justify-between";
const CLS_GRID =
  "grid grid-cols-2 gap-x-[32px] gap-y-[15px] p-5 pt-[20px] pb-[12px]";
const CLS_ALT_BTN =
  "w-[99px] h-[55px] bg-[#D6D6D6] text-black font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-95 rounded-none border-0";
const CLS_ALT_BTN_ACTIVE =
  "w-[99px] h-[55px] bg-[#1BFF16] text-black font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-95 rounded-none border-0";
const CLS_CUSTOM_INPUT =
  "w-full h-[54px] bg-[#FDFDFD] border-0 text-black font-semibold text-[28px] text-center shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none rounded-none my-4";
const CLS_BOTTOM_ROW =
  "flex items-center justify-around px-[24px] pb-[24px] pt-[8px]";
const CLS_BOTTOM_BTN =
  "w-[99px] h-[55px] bg-[#3F3F3F] text-white font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90 rounded-none border-0";

/** Altitude in feet. Returns 0–50000 if valid, otherwise undefined. */
function parseAlt(s: string): number | undefined {
  if (s.trim() === "") return undefined;
  const n = parseInt(s, 10);
  if (Number.isNaN(n) || n < 0 || n > 50000) return undefined;
  return n;
}

interface AltSelectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: number | undefined | null;
  onSelect: (altitude: number | undefined) => void;
}

export function AltSelectDialog({
  open,
  onOpenChange,
  value,
  onSelect,
}: AltSelectDialogProps) {
  const currentAlt = value && value > 0 ? value : undefined;
  const transitionAltitude = useTransitionAltitude();
  const [customInput, setCustomInput] = useState("");
  const [customInvalid, setCustomInvalid] = useState(false);
  const [prevOpen, setPrevOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  if (prevOpen !== open) {
    setPrevOpen(open);
    if (open) {
      const presetMatch =
        currentAlt != null && ALT_PRESET_VALUES.includes(currentAlt);
      setCustomInput(
        currentAlt != null && !presetMatch ? String(currentAlt) : ""
      );
      setCustomInvalid(false);
    }
  }

  useEffect(() => {
    if (open) {
      inputRef.current?.focus();
    }
  }, [open]);

  function handleSelect(alt: number) {
    onSelect(alt);
    onOpenChange(false);
  }

  function handleErase() {
    onSelect(undefined);
    onOpenChange(false);
  }

  function handleCustomSubmit() {
    const parsed = parseAlt(customInput.trim());
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
        <DialogTitle className="sr-only">Select altitude</DialogTitle>
        <div className={CLS_PANEL}>
          <div className={CLS_GRID}>
            {ALT_PRESET_VALUES.map((alt) => (
              <button
                key={alt}
                type="button"
                className={
                  currentAlt === alt ? CLS_ALT_BTN_ACTIVE : CLS_ALT_BTN
                }
                onClick={() => handleSelect(alt)}
              >
                {formatAltitude(alt, transitionAltitude)}
              </button>
            ))}
          </div>
          <div className="px-5">
            <input
              ref={inputRef}
              type="text"
              inputMode="numeric"
              pattern="[0-9]*"
              maxLength={5}
              placeholder="e.g. 3500"
              value={customInput}
              onChange={(e) => {
                const v = e.target.value.replace(/\D/g, "").slice(0, 5);
                setCustomInput(v);
                setCustomInvalid(false);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleCustomSubmit();
              }}
              className={CLS_CUSTOM_INPUT}
              style={{
                fontFamily: "Rubik, Arial, sans-serif",
                ...(customInvalid
                  ? { border: "2px solid #b91c1c", boxShadow: "0 0 0 1px #b91c1c" }
                  : {}),
              }}
              aria-invalid={customInvalid}
              aria-describedby={customInvalid ? "alt-custom-error" : undefined}
            />
            {customInvalid && (
              <p id="alt-custom-error" className="text-red-700 text-sm mt-1 text-center">
                Enter 0–50000 ft
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
