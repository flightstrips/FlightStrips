import type { CSSProperties } from "react";
import { useEffect, useRef, useState } from "react";

import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { formatAltitude } from "@/lib/utils";
import { useTransitionAltitude } from "@/store/store-hooks";
import { scalePx, toDvh, toVw } from "@/lib/viewportScale";

const ALT_PRESET_VALUES = [1500, 2500, 3000, 4000, 5000, 7000];

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

const altButtonStyle: CSSProperties = {
  width: toVw(99),
  height: toDvh(55),
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
  height: toDvh(55),
  background: "#3F3F3F",
  color: "white",
  fontWeight: 600,
  fontSize: toVw(28),
  boxShadow: DIALOG_SHADOW,
  outline: "none",
  borderRadius: 0,
  border: 0,
};

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
        currentAlt != null && !presetMatch ? String(currentAlt) : "",
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
      <DialogContent
        className={dialogContentClassName}
        style={{ width: toVw(301), backgroundColor: "#B3B3B3", border: "1px solid black" }}
      >
        <DialogTitle className="sr-only">Select altitude</DialogTitle>
        <div style={panelStyle}>
          <div style={gridStyle}>
            {ALT_PRESET_VALUES.map((alt) => (
              <button
                key={alt}
                type="button"
                className="active:brightness-95"
                style={{ ...altButtonStyle, background: currentAlt === alt ? "#1BFF16" : "#D6D6D6" }}
                onClick={() => handleSelect(alt)}
              >
                {formatAltitude(alt, transitionAltitude)}
              </button>
            ))}
          </div>
          <div style={{ paddingInline: scalePx(20) }}>
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
              style={{
                ...customInputStyle,
                fontFamily: "Rubik, Arial, sans-serif",
                ...(customInvalid
                  ? { border: "2px solid #b91c1c", boxShadow: "0 0 0 1px #b91c1c" }
                  : {}),
              }}
              aria-invalid={customInvalid}
              aria-describedby={customInvalid ? "alt-custom-error" : undefined}
            />
            {customInvalid && (
              <p
                id="alt-custom-error"
                className="text-red-700 text-center"
                style={{ fontSize: scalePx(14), marginTop: scalePx(4) }}
              >
                Enter 0–50000 ft
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
