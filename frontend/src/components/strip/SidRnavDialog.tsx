import { useEffect, useState, type CSSProperties } from "react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import type { RnavCapability } from "@/lib/rnav";

// Laid out on the 487x753 design frame and scaled with dvh (1080 px base).
const px = (n: number) => `${(n / 10.8).toFixed(3)}dvh`;

const BEVEL: CSSProperties = {
  border: `${px(2)} solid`,
  borderColor: "#CECECE #393939 #393939 #CECECE",
  boxShadow: "inset 2px 2px 2px -1px rgba(206,206,206,0.55), inset -2px -2px 2px -1px rgba(57,57,57,0.55)",
  boxSizing: "border-box",
};

const BTN_BASE: CSSProperties = {
  ...BEVEL,
  fontFamily: "var(--font-bay)",
  fontWeight: 700,
  fontSize: px(22),
  lineHeight: 1.05,
  color: "black",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  textAlign: "center",
  outline: "none",
};

const FILL_NORMAL = "#D6D6D6";
const FILL_ACTIVE = "#1BFF16";
const FILL_DARK = "#3F3F3F";

const LEFT_X = 42;
const LEFT_W = 202;
const RIGHT_X = 288;
const RIGHT_W = 155;
const ROW_PITCH = 60.5;
const ROW_H = 48;
const LIST_TOP = 38;
const LIST_H = 593;

interface SidRnavDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sid: string | undefined | null;
  sids: string[];
  capabilities: string | undefined | null;
  rnavFault: boolean;
  onSelectSid: (sid: string) => void;
  onSelectRnav: (capability: RnavCapability) => void;
  onErase: () => void;
}

export function SidRnavDialog({
  open,
  onOpenChange,
  sid,
  sids,
  capabilities,
  rnavFault,
  onSelectSid,
  onSelectRnav,
  onErase,
}: SidRnavDialogProps) {
  const currentSid = sid?.trim().toUpperCase() ?? "";
  const [pendingSid, setPendingSid] = useState<string | null>(null);
  const [pendingRnav, setPendingRnav] = useState<RnavCapability | null>(null);

  useEffect(() => {
    if (open) {
      setPendingSid(null);
      setPendingRnav(null);
    }
  }, [open]);

  const removeEnabled = rnavFault || capabilities === "NIL" || !capabilities;
  const shownSid = (pendingSid ?? currentSid).toUpperCase();
  const toggleRnav = (value: RnavCapability) => setPendingRnav(p => (p === value ? null : value));

  const handleEsc = () => {
    if (pendingSid !== null) onSelectSid(pendingSid);
    if (pendingRnav !== null && (pendingRnav === "NIL" || removeEnabled)) onSelectRnav(pendingRnav);
    onOpenChange(false);
  };

  const rightButton = (row: number, label: string, onClick: () => void, active = false, disabled = false) => (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      style={{
        ...BTN_BASE,
        position: "absolute",
        left: px(RIGHT_X),
        top: px(LIST_TOP + row * ROW_PITCH),
        width: px(RIGHT_W),
        height: px(ROW_H),
        background: active ? FILL_ACTIVE : FILL_NORMAL,
        opacity: disabled ? 0.45 : 1,
        cursor: disabled ? "default" : "pointer",
      }}
    >
      {label}
    </button>
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-none gap-0 overflow-hidden border border-black bg-[#B3B3B3] p-0 [&>button]:hidden"
        style={{ width: px(487), height: px(753) }}
      >
        <DialogTitle className="sr-only">Select SID or RNAV</DialogTitle>
        <div
          className="absolute border border-black"
          style={{ left: px(12), top: px(17), right: px(12), bottom: px(17) }}
        />

        <div
          className="absolute overflow-y-auto"
          style={{ left: px(LEFT_X), top: px(LIST_TOP), width: px(LEFT_W), height: px(LIST_H), scrollbarWidth: "none" }}
        >
          {sids.map((name, i) => (
            <button
              key={name}
              type="button"
              onClick={() => setPendingSid(name)}
              style={{
                ...BTN_BASE,
                position: "absolute",
                left: 0,
                top: px(i * ROW_PITCH),
                width: px(LEFT_W),
                height: px(ROW_H),
                background: name.toUpperCase() === shownSid ? FILL_ACTIVE : FILL_NORMAL,
                cursor: "pointer",
              }}
            >
              {name}
            </button>
          ))}
          <div style={{ height: px(sids.length * ROW_PITCH), width: 1 }} />
        </div>

        {rightButton(2, "VFR", () => setPendingSid("VFR"), shownSid === "VFR")}
        {rightButton(3, "SA 3000'", () => setPendingSid("SA 3000'"), shownSid === "SA 3000'")}
        {rightButton(7, "NON RNAV", () => toggleRnav("NIL"), pendingRnav ? pendingRnav === "NIL" : capabilities === "NIL")}
        {rightButton(8, "REMOVE NON RNAV", () => toggleRnav("1"), pendingRnav === "1", !removeEnabled)}

        <button
          type="button"
          onClick={() => { onErase(); onOpenChange(false); }}
          style={{
            ...BTN_BASE,
            position: "absolute",
            left: px(LEFT_X),
            top: px(660),
            width: px(LEFT_W),
            height: px(52),
            background: FILL_DARK,
            color: "white",
            cursor: "pointer",
          }}
        >
          ERASE
        </button>
        <button
          type="button"
          onClick={handleEsc}
          style={{
            ...BTN_BASE,
            position: "absolute",
            left: px(RIGHT_X),
            top: px(660),
            width: px(RIGHT_W),
            height: px(52),
            background: FILL_DARK,
            color: "white",
            cursor: "pointer",
          }}
        >
          ESC
        </button>
      </DialogContent>
    </Dialog>
  );
}
