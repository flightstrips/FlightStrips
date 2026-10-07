import { SEMI_BOLD_STROKE } from "./shared";
import { useEffect, useRef, useState, type CSSProperties, type MouseEvent as ReactMouseEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { TacticalStrip } from "@/api/models";
import { useControllers, useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { STRIP_CONTEXT_MENU_WIDTH } from "./StripContextMenu";
import { FONT, SELECTION_COLOR, getFlatStripBorderStyle, getSIBoxBorderStyle } from "./shared";

const HEIGHT = "2.596dvh";
const W_BTN = "1.25vw";
// 90% of the share the SI box takes on flight strips: 8 / (8 + 25 + 3 x 25*2/3 + 25*4/9).
const SI_WIDTH_PERCENT = (8 / (8 + 25 + 3 * ((25 * 2) / 3) + (25 * 4) / 9)) * 100 * 0.9 * 0.9;
const TACTICAL_MENU_HEIGHT = 254;
const MENU_VIEWPORT_MARGIN = 8;
const MENU_BG = "#B3B3B3";
const MENU_ITEM_BG = "#D6D6D6";
const MENU_DISABLED = "#A4A4A4";
const MENU_FONT = "var(--font-bay)";
const MENU_SHADOW = "0 4px 4px rgba(0,0,0,0.25)";

const menuItemStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  backgroundColor: MENU_ITEM_BG,
  color: "black",
  fontFamily: MENU_FONT,
  fontWeight: "normal", WebkitTextStroke: SEMI_BOLD_STROKE,
  fontSize: 16,
  cursor: "pointer",
  userSelect: "none",
  boxShadow: MENU_SHADOW,
};

interface TacticalStripShellProps {
  strip: TacticalStrip;
  width?: string | number;
  backgroundColor: string;
  borderColor: string;
  textColor: string;
  children: ReactNode;
  action?: ReactNode;
  deleteHoverClass: string;
}

export function TacticalActionCell({
  color,
  children,
  onClick,
  clickable = false,
  width = W_BTN,
  ariaLabel,
}: {
  color: string;
  children: ReactNode;
  onClick?: () => void;
  clickable?: boolean;
  width?: string;
  ariaLabel?: string;
}) {
  const handleClick = (event: ReactMouseEvent<HTMLDivElement>) => {
    event.stopPropagation();
    onClick?.();
  };

  return (
    <div
      className="flex-shrink-0 flex items-center justify-center"
      role={clickable ? "button" : undefined}
      aria-label={ariaLabel}
      tabIndex={clickable ? 0 : undefined}
      style={{
        width,
        height: "100%",
        color,
        cursor: clickable ? "pointer" : "default",
      }}
      onClick={handleClick}
      onKeyDown={clickable ? (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          event.stopPropagation();
          onClick?.();
        }
      } : undefined}
    >
      {children}
    </div>
  );
}

function TacticalOwnershipMenu({
  strip,
  position,
  onClose,
}: {
  strip: TacticalStrip;
  position: { x: number; y: number };
  onClose: () => void;
}) {
  const controllers = useControllers();
  const forceAssume = useWebSocketStore((state) => state.forceAssumeTacticalStrip);
  const menuRef = useRef<HTMLDivElement>(null);
  const ownerIdentifier = controllers.find((controller) => controller.position === strip.owner)?.identifier || strip.owner;
  const menuX = Math.min(position.x, window.innerWidth - STRIP_CONTEXT_MENU_WIDTH - MENU_VIEWPORT_MARGIN);
  const menuY = Math.min(position.y, window.innerHeight - TACTICAL_MENU_HEIGHT - MENU_VIEWPORT_MARGIN);

  useEffect(() => {
    function handleMouseDown(event: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        onClose();
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        onClose();
      }
    }

    document.addEventListener("mousedown", handleMouseDown);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handleMouseDown);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [onClose]);

  const handleForceAssume = () => {
    forceAssume(strip.id);
    onClose();
  };

  return createPortal(
    <div
      ref={menuRef}
      role="dialog"
      aria-label="Tactical strip actions"
      style={{
        position: "fixed",
        left: menuX,
        top: menuY,
        width: STRIP_CONTEXT_MENU_WIDTH,
        boxSizing: "border-box",
        backgroundColor: MENU_BG,
        border: "1px solid black",
        zIndex: 9999,
        display: "flex",
        flexDirection: "column",
        padding: "13px 7px",
        gap: 4,
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          height: 25,
          flexShrink: 0,
          backgroundColor: "white",
          color: "#AFAFAF",
          fontFamily: MENU_FONT,
          fontWeight: 300,
          fontSize: 16,
          userSelect: "none",
          boxShadow: MENU_SHADOW,
        }}
      >
        {ownerIdentifier || "—"}
      </div>

      <button
        style={{ ...menuItemStyle, height: 50, flexShrink: 0, border: 0 }}
        onClick={handleForceAssume}
      >
        FORCE ASSUME
      </button>

      <button
        style={{
          ...menuItemStyle,
          height: 48,
          flexShrink: 0,
          border: 0,
          color: MENU_DISABLED,
          cursor: "not-allowed",
        }}
        disabled
      >
        RECALL
      </button>

      <button
        style={{
          ...menuItemStyle,
          height: 44,
          flexShrink: 0,
          border: 0,
          color: MENU_DISABLED,
          cursor: "not-allowed",
        }}
        disabled
      >
        VIEW FPL
      </button>

      <button
        style={{
          ...menuItemStyle,
          height: 43,
          flexShrink: 0,
          border: 0,
          backgroundColor: "#3F3F3F",
          color: "white",
          fontSize: 18,
        }}
        onClick={onClose}
      >
        ESC
      </button>
    </div>,
    document.body,
  );
}

export function TacticalStripShell({
  strip,
  width,
  backgroundColor,
  borderColor,
  textColor,
  children,
  action,
  deleteHoverClass,
}: TacticalStripShellProps) {
  const [menuPosition, setMenuPosition] = useState<{ x: number; y: number } | null>(null);
  const myPosition = useMyPosition();
  const deleteTacticalStrip = useWebSocketStore((state) => state.deleteTacticalStrip);
  const markTacticalStrip = useWebSocketStore((state) => state.markTacticalStrip);
  const isOwner = strip.owner === myPosition;

  const handleStripClick = (event: ReactMouseEvent<HTMLDivElement>) => {
    event.stopPropagation();
    if (isOwner) {
      markTacticalStrip(strip.id, !strip.marked);
      return;
    }
    setMenuPosition({ x: event.clientX, y: event.clientY });
  };

  return (
    <>
      <div
        className="flex select-none"
        style={{
          height: HEIGHT,
          width: width ?? "100%",
          backgroundColor,
          ...getFlatStripBorderStyle(backgroundColor, backgroundColor),
          cursor: "pointer",
        }}
        onClick={handleStripClick}
      >
        {isOwner && (
          <div
            className="flex-shrink-0 bg-white"
            style={{ height: "100%", flex: `0 0 ${SI_WIDTH_PERCENT}%`, ...getSIBoxBorderStyle(false, borderColor) }}
          />
        )}

        <div
          className="flex-1 flex items-center justify-center px-[0.42vw] overflow-hidden font-normal"
          style={{
            fontFamily: FONT,
            color: textColor,
            fontSize: "0.945vw",
            backgroundColor: strip.marked ? SELECTION_COLOR : undefined,
          }}
        >
          <span className="truncate">{children}</span>
        </div>

        {action}

        {isOwner && (
          <div
            className={`flex-shrink-0 flex items-center justify-center cursor-pointer ${deleteHoverClass}`}
            style={{ width: W_BTN, height: "100%", color: textColor }}
            onClick={(event) => {
              event.stopPropagation();
              deleteTacticalStrip(strip.id);
            }}
          >
            <span style={{ fontFamily: FONT, fontSize: "1.1dvh", lineHeight: 1, width: "1.7dvh", height: "1.7dvh", flexShrink: 0, display: "flex", alignItems: "center", justifyContent: "center", border: "1px solid currentColor", borderRadius: "0.4dvh", boxSizing: "border-box" }}>✕</span>
          </div>
        )}
      </div>

      {!isOwner && menuPosition && (
        <TacticalOwnershipMenu strip={strip} position={menuPosition} onClose={() => setMenuPosition(null)} />
      )}
    </>
  );
}
