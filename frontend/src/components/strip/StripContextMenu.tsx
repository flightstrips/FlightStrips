import { useEffect, useRef, useState } from "react";
import { useStrip, useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import FlightPlanDialog from "@/components/FlightPlanDialog";

export interface StripContextMenuProps {
  callsign: string;
  position: { x: number; y: number };
  onClose: () => void;
}

// From design SVG: 167px wide panel
const MENU_W = 167;
const MENU_H_APPROX = 420;

// Colours from design SVG
const COLOR_PANEL_BG  = "#B3B3B3"; // outer panel
const COLOR_ITEM_BG   = "#D6D6D6"; // button cards
const COLOR_ESC_BG    = "#3F3F3F"; // ESC button
const COLOR_DISABLED  = "#A4A4A4"; // greyed text (disabled)
const FONT            = "'Arial', sans-serif";

/** Drop shadow matching design filters (drop shadow dy=4, blur=2, opacity=0.25). */
const DROP_SHADOW = "0 4px 4px rgba(0,0,0,0.25)";

/** Base style for interactive button rows. */
const itemStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  backgroundColor: COLOR_ITEM_BG,
  color: "black",
  fontFamily: FONT,
  fontWeight: 600,
  fontSize: 16,
  cursor: "pointer",
  userSelect: "none",
  boxShadow: DROP_SHADOW,
};

const disabledStyle: React.CSSProperties = {
  ...itemStyle,
  color: COLOR_DISABLED,
  cursor: "not-allowed",
};

/** Simple SVG person silhouette — matches the image in the design. */
function ManIcon() {
  return (
    <svg width="14" height="37" viewBox="0 0 14 37" fill="black" aria-hidden="true">
      <ellipse cx="7" cy="6" rx="5" ry="6" />
      <path d="M0 22c0-4 3-8 7-8s7 4 7 8v15H0V22z" />
    </svg>
  );
}

export function StripContextMenu({ callsign, position, onClose }: StripContextMenuProps) {
  const strip = useStrip(callsign);
  const myPosition = useWebSocketStore((s) => s.position);
  const stripTransfers = useStripTransfers();
  const forceAssumeStrip = useWebSocketStore((s) => s.forceAssumeStrip);
  const cancelTransfer = useWebSocketStore((s) => s.cancelTransfer);
  const updateStrip = useWebSocketStore((s) => s.updateStrip);

  const [showFpl, setShowFpl] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // FORCE ASSUME: disabled if you already own the strip or there is an active transfer
  const forceAssumeDisabled = strip?.owner === myPosition || !!stripTransfers[callsign];

  // RECALL: enabled when I am the owner and there's an outgoing transfer
  const recallDisabled = !(
    myPosition &&
    strip?.owner === myPosition &&
    stripTransfers[callsign] !== undefined
  );

  // Clamp menu to viewport
  const menuX = Math.min(position.x, window.innerWidth - MENU_W - 8);
  const menuY = Math.min(position.y, window.innerHeight - MENU_H_APPROX - 8);

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        onClose();
      }
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("mousedown", onMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [onClose]);

  const handleObToggle = () => {
    if (!strip) return;
    updateStrip(callsign, { ob: !strip.ob });
    onClose();
  };

  const handleForceAssume = () => {
    if (forceAssumeDisabled) return;
    forceAssumeStrip(callsign);
    onClose();
  };

  const handleRecall = () => {
    if (recallDisabled) return;
    cancelTransfer(callsign);
    onClose();
  };

  // When showing FPL, render only the dialog — menu unmounts when dialog closes
  if (showFpl) {
    return (
      <FlightPlanDialog
        callsign={callsign}
        mode="view"
        open={true}
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
      />
    );
  }

  const isOb = strip?.ob ?? false;

  return (
    <div
      ref={menuRef}
      style={{
        position: "fixed",
        left: menuX,
        top: menuY,
        width: MENU_W,
        backgroundColor: COLOR_PANEL_BG,
        border: "1px solid black",
        zIndex: 9999,
        display: "flex",
        flexDirection: "column",
        // Inner frame padding — matches the inset rect in SVG (7.5px sides, ~13px top)
        padding: "13px 7px 13px 7px",
        gap: 4,
      }}
    >
      {/* OWNER — white card, light-weight grey text, read-only */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          height: 25,
          backgroundColor: "white",
          color: "#AFAFAF",
          fontFamily: FONT,
          fontWeight: 300,
          fontSize: 16,
          userSelect: "none",
          boxShadow: DROP_SHADOW,
        }}
      >
        {strip?.owner || "—"}
      </div>

      {/* OB / IB — inset shadow when active (toggled state) */}
      <div
        style={{
          ...itemStyle,
          height: 23,
          // Inset shadow matches filter4_i in design (inner shadow dy=4, blur=2)
          boxShadow: isOb
            ? "inset 0 4px 4px rgba(0,0,0,0.25)"
            : DROP_SHADOW,
        }}
        onClick={handleObToggle}
      >
        {isOb ? "IB" : "OB"}
      </div>

      {/* FORCE ASSUME — ~50px tall */}
      <div
        style={{
          ...(forceAssumeDisabled ? disabledStyle : itemStyle),
          height: 50,
        }}
        onClick={forceAssumeDisabled ? undefined : handleForceAssume}
      >
        FORCE ASSUME
      </div>

      {/* Man image — decorative, ~50px tall, no action */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          height: 50,
          backgroundColor: COLOR_ITEM_BG,
          boxShadow: DROP_SHADOW,
        }}
        aria-hidden="true"
      >
        <ManIcon />
      </div>

      {/* RECALL — ~48px tall */}
      <div
        style={{
          ...(recallDisabled ? disabledStyle : itemStyle),
          height: 48,
        }}
        onClick={recallDisabled ? undefined : handleRecall}
      >
        RECALL
      </div>

      {/* VIEW FPL — ~44px tall, black text (enabled) */}
      <div
        style={{ ...itemStyle, height: 44 }}
        onClick={() => setShowFpl(true)}
      >
        VIEW FPL
      </div>

      {/* Spacer before ESC — matches the ~38px gap in design */}
      <div style={{ height: 34, flexShrink: 0 }} />

      {/* ESC — dark background, white text, font-size 18 */}
      <div
        style={{
          ...itemStyle,
          height: 43,
          backgroundColor: COLOR_ESC_BG,
          color: "white",
          fontSize: 18,
        }}
        onClick={onClose}
      >
        ESC
      </div>
    </div>
  );
}

