import { useEffect, useRef, useState, type CSSProperties, type MouseEvent as ReactMouseEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { TacticalStrip } from "@/api/models";
import { useControllers, useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { STRIP_CONTEXT_MENU_WIDTH } from "./StripContextMenu";
import { FONT, SELECTION_COLOR, getFlatStripBorderStyle } from "./shared";

const HEIGHT = "2.36dvh";
const W_SI = "1.77vw";
const W_BTN = "1.25vw";
const TACTICAL_MENU_HEIGHT = 254;
const MENU_VIEWPORT_MARGIN = 8;
const MENU_BG = "#B3B3B3";
const MENU_ITEM_BG = "#D6D6D6";
const MENU_DISABLED = "#A4A4A4";
const MENU_FONT = "'Arial', sans-serif";
const MENU_SHADOW = "0 4px 4px rgba(0,0,0,0.25)";

const menuItemStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  backgroundColor: MENU_ITEM_BG,
  color: "black",
  fontFamily: MENU_FONT,
  fontWeight: 600,
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
  borderColor,
  color,
  children,
  onClick,
  clickable = false,
}: {
  borderColor: string;
  color: string;
  children: ReactNode;
  onClick?: () => void;
  clickable?: boolean;
}) {
  const handleClick = (event: ReactMouseEvent<HTMLDivElement>) => {
    event.stopPropagation();
    onClick?.();
  };

  return (
    <div
      className="flex-shrink-0 flex items-center justify-center border-l-2"
      style={{
        width: W_BTN,
        height: "100%",
        borderLeftColor: borderColor,
        color,
        cursor: clickable ? "pointer" : "default",
      }}
      onClick={handleClick}
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
          ...getFlatStripBorderStyle({ borderBottom: `1px solid ${borderColor}` }),
          cursor: "pointer",
        }}
        onClick={handleStripClick}
      >
        {isOwner && (
          <div
            className="flex-shrink-0 border-r-2 bg-white"
            style={{ width: W_SI, height: "100%", borderRightColor: borderColor }}
          />
        )}

        <div
          className="flex-1 flex items-center justify-center px-[0.42vw] overflow-hidden font-bold"
          style={{
            fontFamily: FONT,
            color: textColor,
            fontSize: "0.63vw",
            backgroundColor: strip.marked ? SELECTION_COLOR : undefined,
          }}
        >
          <span className="truncate">{children}</span>
        </div>

        {action}

        {isOwner && (
          <div
            className={`flex-shrink-0 flex items-center justify-center border-l-2 cursor-pointer ${deleteHoverClass}`}
            style={{ width: W_BTN, height: "100%", borderLeftColor: borderColor, color: textColor }}
            onClick={(event) => {
              event.stopPropagation();
              deleteTacticalStrip(strip.id);
            }}
          >
            <span style={{ fontFamily: FONT, fontSize: "0.68vw" }}>✕</span>
          </div>
        )}
      </div>

      {!isOwner && menuPosition && (
        <TacticalOwnershipMenu strip={strip} position={menuPosition} onClose={() => setMenuPosition(null)} />
      )}
    </>
  );
}
