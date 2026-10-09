import { useEffect, useRef, useState } from "react";
import { useIsClrDel, useStrip, useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import { canForceAssumeStrip } from "./shared";
import { EST_RAISED_EDGE, EST_SUNKEN_EDGE } from "@/components/est/bevel";
import { useIsLocallyHidden, useLocalHiddenStrips } from "@/store/localHiddenStrips";

export interface StripContextMenuProps {
  callsign: string;
  position: { x: number; y: number };
  onClose: () => void;
}

// From design SVG: 167px wide panel
export const STRIP_CONTEXT_MENU_WIDTH = 107;
export const STRIP_CONTEXT_MENU_HEIGHT = 401;

// Colours from design SVG
const COLOR_PANEL_BG  = "#B3B3B3"; // outer panel
const COLOR_ITEM_BG   = "#D6D6D6"; // button cards
const COLOR_ESC_BG    = "#3F3F3F"; // ESC button
const COLOR_DISABLED  = "#A4A4A4"; // greyed text (disabled)
const FONT            = "var(--font-bay)";

/** Drop shadow matching design filters (drop shadow dy=4, blur=2, opacity=0.25). */
const DROP_SHADOW = "0 4px 4px rgba(0,0,0,0.25)";

const RAISED_3D: React.CSSProperties = { ...EST_RAISED_EDGE, boxShadow: `${EST_RAISED_EDGE.boxShadow}, ${DROP_SHADOW}` };
const SUNKEN_3D: React.CSSProperties = { ...EST_SUNKEN_EDGE };

/** Base style for interactive button rows. */
const itemStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  backgroundColor: COLOR_ITEM_BG,
  color: "black",
  fontFamily: FONT,
  fontWeight: 600,
  fontSize: 14,
  lineHeight: 1.1,
  textAlign: "center",
  cursor: "pointer",
  userSelect: "none",
  ...RAISED_3D,
};

const disabledStyle: React.CSSProperties = {
  ...itemStyle,
  color: COLOR_DISABLED,
  cursor: "not-allowed",
};

/** Simple SVG person silhouette — matches the image in the design. */
function ManIcon() {
  return (
    <svg width="15" height="40" viewBox="0 0 24 64" fill="black" aria-hidden="true" shapeRendering="crispEdges">
      <circle cx="12" cy="6" r="5" shapeRendering="geometricPrecision" />
      <path d="M4 13h16l2 2v26H2V15z" />
      <rect x="8" y="41" width="8" height="23" />
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

  const isClrDel = useIsClrDel();
  const localHidden = useIsLocallyHidden(callsign);
  const setLocalHidden = useLocalHiddenStrips((s) => s.setHidden);
  const [showFpl, setShowFpl] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // FORCE ASSUME: disabled in CLR DEL (never owns strips), or when already
  // owning the strip. It remains available for unowned strips as a recovery
  // action.
  const forceAssumeDisabled = !canForceAssumeStrip({
    owner: strip?.owner,
    myPosition,
    isClrDel,
  });

  const activeTransfer = stripTransfers[callsign];

  // RECALL: enabled only for the controller that initiated the active coordination.
  const recallDisabled = !(
    myPosition &&
    activeTransfer !== undefined &&
    activeTransfer.from === myPosition
  );

  // Clamp menu to viewport
  const menuX = Math.min(position.x, window.innerWidth - STRIP_CONTEXT_MENU_WIDTH - 8);
  const menuY = Math.min(position.y, window.innerHeight - STRIP_CONTEXT_MENU_HEIGHT - 8);

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
        width: STRIP_CONTEXT_MENU_WIDTH,
        height: STRIP_CONTEXT_MENU_HEIGHT,
        boxSizing: "border-box",
        backgroundColor: COLOR_PANEL_BG,
        border: "1px solid black",
        zIndex: 9999,
        display: "flex",
        flexDirection: "column",
        // Inner frame padding — matches the inset rect in SVG (7.5px sides, ~13px top)
        padding: "13px 4px 13px 4px",
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
          fontSize: 14,
          overflow: "hidden",
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
          ...(isOb ? SUNKEN_3D : RAISED_3D),
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
          ...RAISED_3D,
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

      {/* LCL HIDE / LCL SHOW — local-only half-strip toggle */}
      <div
        style={{ ...itemStyle, height: 44 }}
        onClick={() => {
          setLocalHidden(callsign, !localHidden);
          onClose();
        }}
      >
        {localHidden ? "LCL SHOW" : "LCL HIDE"}
      </div>

      <div style={{ flex: 1, minHeight: 0 }} />

      {/* ESC — dark background, white text, font-size 18 */}
      <div
        style={{
          ...itemStyle,
          height: 43,
          backgroundColor: COLOR_ESC_BG,
          color: "white",
          fontSize: 16,
        }}
        onClick={onClose}
      >
        ESC
      </div>
    </div>
  );
}

