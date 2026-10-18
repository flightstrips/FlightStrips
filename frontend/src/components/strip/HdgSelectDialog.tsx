import { useEffect, useState } from "react";
import type { CSSProperties } from "react";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { toDvh, toVw } from "@/lib/viewportScale";

export type HdgTurnDirection = "left" | "right";

const SA_HEADINGS: Record<string, number> = {
  "04": 27,
  "12": 119,
  "22": 217,
  "30": 299,
};
const HEADING_PRESETS: { label: string; heading: number; direction: HdgTurnDirection }[] = [
  { label: "LT350", heading: 350, direction: "left" },
  { label: "LT360", heading: 360, direction: "left" },
  { label: "RT090", heading: 90, direction: "right" },
];
const KEYPAD_ROWS = [
  ["LT", "SA", "RT"],
  ["7", "8", "9"],
  ["4", "5", "6"],
  ["1", "2", "3"],
  ["0", "00"],
];

const RAISED: CSSProperties = {
  border: "2px solid",
  borderColor: "#CECECE #666666 #666666 #CECECE",
  boxShadow: "inset 2px 2px 2px -1px rgba(206,206,206,0.55), inset -2px -2px 2px -1px rgba(102,102,102,0.55)",
  boxSizing: "border-box",
};
const INSET: CSSProperties = {
  border: "2px solid",
  borderColor: "#666666 #CECECE #CECECE #666666",
  boxShadow: "inset 2px 2px 2px -1px rgba(102,102,102,0.55), inset -2px -2px 2px -1px rgba(206,206,206,0.55)",
  boxSizing: "border-box",
};
const dialogContentClassName = "max-w-none max-h-none gap-0 overflow-hidden p-0 [&>button]:hidden";
const panelStyle: CSSProperties = {
  margin: `${toDvh(16)} ${toVw(14)}`,
  border: "1px solid black",
  padding: `${toDvh(12)} ${toVw(18)}`,
  display: "flex",
  flexDirection: "column",
  justifyContent: "space-between",
};
const topRowStyle: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "1fr 1.2fr 1fr",
  gap: toVw(9),
  alignItems: "center",
};
const topButtonStyle: CSSProperties = {
  height: toDvh(70),
  background: "#3F3F3F",
  color: "white",
  fontWeight: 700,
  fontSize: toVw(26),
  ...RAISED,
  borderRadius: 0,
};
const entryStyle: CSSProperties = {
  width: "100%",
  height: toDvh(70),
  background: "#FDFDFD",
  color: "black",
  fontWeight: 600,
  fontSize: toVw(34),
  textAlign: "center",
  ...INSET,
  outline: "none",
  borderRadius: 0,
};
const entryAreaStyle: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "3fr 1fr",
  gap: toVw(14),
  alignItems: "center",
  flex: 1,
};
const keypadStyle: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(3, 1fr)",
  gap: `${toDvh(10)} ${toVw(12)}`,
  padding: `${toDvh(18)} ${toVw(4)}`,
};
const keyStyle: CSSProperties = {
  height: toDvh(68),
  background: "#D6D6D6",
  color: "black",
  fontWeight: 700,
  fontSize: toVw(28),
  ...RAISED,
  borderRadius: 0,
};
const presetColumnStyle: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: toDvh(10),
};
const presetButtonStyle: CSSProperties = {
  ...keyStyle,
  height: toDvh(68),
  paddingInline: toVw(5),
  fontSize: toVw(18),
};
const footerStyle: CSSProperties = {
  display: "flex",
  justifyContent: "space-between",
  padding: `${toDvh(12)} ${toVw(18)} ${toDvh(8)}`,
};
const footerButtonStyle: CSSProperties = {
  width: toVw(140),
  height: toDvh(68),
  background: "#3F3F3F",
  color: "white",
  fontWeight: 700,
  fontSize: toVw(28),
  ...RAISED,
  borderRadius: 0,
};

interface HdgSelectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: number | undefined | null;
  onSelect: (heading: number | undefined) => void;
  runway?: string;
  turnDirection?: HdgTurnDirection;
  onTurnDirectionChange?: (direction: HdgTurnDirection | undefined) => void;
}

export function HdgSelectDialog({
  open,
  onOpenChange,
  value,
  onSelect,
  runway,
  turnDirection,
  onTurnDirectionChange,
}: HdgSelectDialogProps) {
  const [entry, setEntry] = useState("");
  const [pendingTurnDirection, setPendingTurnDirection] = useState<HdgTurnDirection>();

  useEffect(() => {
    if (open) {
      setEntry(value != null && value > 0 ? String(value).padStart(3, "0") : "");
      setPendingTurnDirection(turnDirection);
    }
  }, [open, value, turnDirection]);

  function append(valueToAdd: string) {
    setEntry((current) => `${current}${valueToAdd}`.slice(0, 3));
  }

  function handleKey(key: string) {
    if (key === "LT") {
      setPendingTurnDirection("left");
    } else if (key === "RT") {
      setPendingTurnDirection("right");
    } else if (key === "SA") {
      const runwayNumber = runway?.match(/^\s*(04|12|22|30)/)?.[1];
      const heading = runwayNumber ? SA_HEADINGS[runwayNumber] : undefined;
      if (heading != null) {
        setEntry(String(heading).padStart(3, "0"));
        setPendingTurnDirection(undefined);
      }
    } else if (key === "00" || /^\d$/.test(key)) {
      append(key);
    }
  }

  function handleSubmit() {
    const heading = entry ? Number(entry) : undefined;
    if (heading != null && heading > 360) return;
    onTurnDirectionChange?.(pendingTurnDirection);
    onSelect(heading);
    onOpenChange(false);
  }

  function handleErase() {
    setEntry("");
    setPendingTurnDirection(undefined);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={dialogContentClassName}
        style={{ width: toVw(600), height: toDvh(680), backgroundColor: "#B3B3B3", border: "1px solid black" }}
      >
        <DialogTitle className="sr-only">Select heading</DialogTitle>
        <div style={panelStyle}>
          <div style={topRowStyle}>
            <button type="button" className="active:brightness-90" style={topButtonStyle} onClick={handleErase}>
              ERASE
            </button>
            <input
              aria-label="Heading"
              inputMode="numeric"
              readOnly
              value={entry}
              style={{ ...entryStyle, fontFamily: "var(--font-bay)" }}
              onKeyDown={(event) => {
                if (event.key === "Enter") handleSubmit();
              }}
            />
            <button
              type="button"
              aria-label="Backspace"
              className="active:brightness-90"
              style={topButtonStyle}
              onClick={() => setEntry((current) => current.slice(0, -1))}
            >
              &lt;--
            </button>
          </div>
          <div style={entryAreaStyle}>
            <div style={keypadStyle}>
              {KEYPAD_ROWS.flatMap((row) =>
                row.map((key) => (
                  <button
                    key={key}
                    type="button"
                    className="active:brightness-95"
                    style={{ ...keyStyle, ...(key === "00" ? { gridColumn: "span 2" } : {}) }}
                    onClick={() => handleKey(key)}
                  >
                    {key}
                  </button>
                )),
              )}
            </div>
            <div style={presetColumnStyle}>
              {HEADING_PRESETS.map(({ label, heading, direction }) => (
                <button
                  key={label}
                  type="button"
                  className="active:brightness-95"
                  style={presetButtonStyle}
                  onClick={() => {
                    setEntry(String(heading).padStart(3, "0"));
                    setPendingTurnDirection(direction);
                  }}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div style={footerStyle}>
            <button
              type="button"
              className="active:brightness-90"
              style={footerButtonStyle}
              onClick={() => onOpenChange(false)}
            >
              ESC
            </button>
            <button
              type="button"
              className="active:brightness-90"
              style={footerButtonStyle}
              onClick={handleSubmit}
            >
              OK
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
