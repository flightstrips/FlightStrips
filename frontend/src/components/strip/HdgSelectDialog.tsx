import { useState, useRef, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import type { CSSProperties } from "react";
import { scalePx, toDvh, toVw } from "@/lib/viewportScale";

// Pre-defined headings from HDG.svg (360, 090, 120, 300, 040, 220) in 2-column layout
const HDG_PRESETS = [360, 90, 120, 300, 40, 220];

const DIALOG_SHADOW = `0 ${scalePx(4)} ${scalePx(4)} rgba(0,0,0,0.25)`;
const dialogContentClassName = "max-w-none max-h-none gap-0 overflow-hidden p-0 [&>button]:hidden";

const panelStyle: CSSProperties = {
  marginLeft: toVw(15),
  marginRight: toVw(15),
  marginTop: toDvh(18),
  border: "1px solid black",
  display: "flex",
  flexDirection: "column",
  justifyContent: "space-between",
};

const gridStyle: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(2, 1fr)",
  justifyItems: "center",
  columnGap: toVw(32),
  rowGap: toDvh(15),
  padding: `${toDvh(20)} ${scalePx(20)} ${toDvh(12)}`,
};

const hdgButtonStyle: CSSProperties = {
  width: toVw(99),
  height: toDvh(54),
  color: "black",
  fontWeight: 600,
  fontSize: toVw(28),
  boxShadow: DIALOG_SHADOW,
  outline: "none",
  borderRadius: 0,
  border: 0,
};

const customInputStyle: CSSProperties = {
  width: "100%",
  height: toDvh(54),
  background: "#FDFDFD",
  border: 0,
  color: "black",
  fontWeight: 600,
  fontSize: toVw(28),
  textAlign: "center",
  boxShadow: DIALOG_SHADOW,
  outline: "none",
  borderRadius: 0,
  marginTop: scalePx(16),
  marginBottom: scalePx(16),
};

const bottomRowStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "space-around",
  padding: `${toDvh(8)} ${toVw(24)} ${toDvh(24)}`,
};

const bottomButtonStyle: CSSProperties = {
  width: toVw(99),
  height: toDvh(54),
  background: "#3F3F3F",
  color: "white",
  fontWeight: 600,
  fontSize: toVw(28),
  boxShadow: DIALOG_SHADOW,
  outline: "none",
  borderRadius: 0,
  border: 0,
};

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
  const currentHdg = value && value > 0 ? value : undefined;
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
      <DialogContent
        className={dialogContentClassName}
        style={{ width: toVw(300), backgroundColor: "#B3B3B3", border: "1px solid black" }}
      >
        <DialogTitle className="sr-only">Select heading</DialogTitle>
        <div style={panelStyle}>
          <div style={gridStyle}>
            {HDG_PRESETS.map((h) => (
              <button
                key={h}
                type="button"
                className="active:brightness-95"
                style={{ ...hdgButtonStyle, background: currentHdg === h ? "#1BFF16" : "#D6D6D6" }}
                onClick={() => handleSelect(h)}
              >
                {formatHdg(h)}
              </button>
            ))}
          </div>
          <div style={{ paddingInline: scalePx(20) }}>
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
              style={{
                ...customInputStyle,
                fontFamily: "Rubik, Arial, sans-serif",
                ...(customInvalid ? { border: "2px solid #b91c1c", boxShadow: "0 0 0 1px #b91c1c" } : {}),
              }}
              aria-invalid={customInvalid}
              aria-describedby={customInvalid ? "hdg-custom-error" : undefined}
            />
            {customInvalid && (
              <p id="hdg-custom-error" className="text-red-700 text-center" style={{ fontSize: scalePx(14), marginTop: scalePx(4) }}>
                Enter 000–360
              </p>
            )}
          </div>
          <div style={bottomRowStyle}>
            <button type="button" className="active:brightness-90" style={bottomButtonStyle} onClick={handleErase}>
              ERASE
            </button>
            <button
              type="button"
              className="active:brightness-90"
              style={bottomButtonStyle}
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
