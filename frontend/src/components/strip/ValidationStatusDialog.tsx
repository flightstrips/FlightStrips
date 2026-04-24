import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import { HoldingPointDialog } from "@/components/map-dialogs/HoldingPointDialog";
import { ArrStandDialog } from "@/components/strip/ArrStandDialog";
import { useStrip, useWebSocketStore } from "@/store/store-hooks";
import type { ValidationStatus } from "@/api/models";
import { useState, type CSSProperties } from "react";

const BASELINE_WIDTH = 1920;
const BASELINE_HEIGHT = 1080;
const DIALOG_SHADOW = "0 min(0.2083vw, 0.3704vh) min(0.2083vw, 0.3704vh) rgba(0,0,0,0.25)";

const toVw = (px: number) => `${(px / BASELINE_WIDTH) * 100}vw`;
const toVh = (px: number) => `${(px / BASELINE_HEIGHT) * 100}vh`;
const toVMin = (px: number) => `min(${toVw(px)}, ${toVh(px)})`;

const dialogContentClassName = "max-w-none max-h-none gap-0 overflow-hidden p-0 [&>button]:hidden";

const rootStyle: CSSProperties = {
  width: toVw(464),
  height: toVh(347),
  maxWidth: "none",
  border: "1px solid #000",
  backgroundColor: "#B3B3B3",
  color: "#000",
  fontFamily: "Rubik, sans-serif",
  padding: 0,
};

const frameLineStyle: CSSProperties = {
  position: "absolute",
  backgroundColor: "#000",
  pointerEvents: "none",
};

const sharedButtonStyle: CSSProperties = {
  position: "absolute",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  backgroundColor: "#3F3F3F",
  color: "#FFF",
  border: "none",
  boxShadow: DIALOG_SHADOW,
  fontFamily: "Rubik, sans-serif",
  fontSize: toVMin(18),
  fontWeight: 600,
  lineHeight: 1.15,
  textAlign: "center",
  padding: 0,
};

const messagePanelStyle: CSSProperties = {
  position: "absolute",
  left: toVw(23.373),
  top: toVh(79.938),
  width: toVw(419.627),
  height: toVh(154),
  backgroundColor: "#D6D6D6",
  boxShadow: DIALOG_SHADOW,
  padding: `${toVh(9)} ${toVw(12)}`,
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "flex-start",
  overflow: "hidden",
};

const customActionLabelLines: Record<string, string[]> = {
  "ASSIGN HP": ["ASSIGN", "HP"],
  "ASSIGN HS": ["ASSIGN", "HS"],
  "ASSIGN NEW": ["ASSIGN", "NEW"],
  "OPEN DCL MENU": ["OPEN", "DCL MENU"],
  "CLEAR TO LAND": ["CLEAR", "TO LAND"],
  "REQ NEW": ["REQ NEW"],
};

function getCustomActionLabelLines(label?: string) {
  const normalizedLabel = label?.trim().toUpperCase() ?? "";

  if (!normalizedLabel) {
    return [];
  }

  if (customActionLabelLines[normalizedLabel]) {
    return customActionLabelLines[normalizedLabel];
  }

  const words = normalizedLabel.split(/\s+/);

  if (words.length > 1) {
    return [words[0], words.slice(1).join(" ")];
  }

  return [normalizedLabel];
}
function getMessageTextStyle(message: string): CSSProperties {
  const normalizedMessage = message.trim();
  const lineCount = normalizedMessage.split("\n").length;
  const length = normalizedMessage.length;

  let fontSize = 14;
  if (lineCount >= 5 || length > 160) fontSize = 13;
  if (lineCount >= 7 || length > 240) fontSize = 12;
  if (lineCount >= 9 || length > 340) fontSize = 11;

  return {
    width: "100%",
    minHeight: 0,
    fontSize: toVMin(fontSize),
    fontWeight: 600,
    lineHeight: lineCount >= 7 ? 1.1 : 1.18,
    whiteSpace: "pre-wrap",
    overflowWrap: "anywhere",
    wordBreak: "break-word",
    overflowY: "auto",
    paddingRight: toVw(4),
  };
}

interface ValidationStatusDialogProps {
  callsign: string;
  status: ValidationStatus;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ValidationStatusDialog({
  callsign,
  status,
  open,
  onOpenChange,
}: ValidationStatusDialogProps) {
  const acknowledgeValidationStatus = useWebSocketStore((state) => state.acknowledgeValidationStatus);
  const generateSquawk = useWebSocketStore((state) => state.generateSquawk);
  const runwayClearance = useWebSocketStore((state) => state.runwayClearance);
  const strip = useStrip(callsign);
  const [holdingPointOpen, setHoldingPointOpen] = useState(false);
  const [flightPlanOpen, setFlightPlanOpen] = useState(false);
  const [standOpen, setStandOpen] = useState(false);
  const validationDialogOpen = open && !holdingPointOpen && !flightPlanOpen && !standOpen;
  const customActionLines = getCustomActionLabelLines(status.custom_action?.label);

  function handleAcknowledge() {
    acknowledgeValidationStatus(callsign, status.activation_key);
    onOpenChange(false);
  }

  function handleCustomAction() {
    if (status.custom_action?.action_kind === "generate_squawk") {
      generateSquawk(callsign);
      onOpenChange(false);
      return;
    }
    if (status.custom_action?.action_kind === "assign_holding_point") {
      setHoldingPointOpen(true);
      onOpenChange(false);
      return;
    }
    if (status.custom_action?.action_kind === "open_dcl_menu") {
      setFlightPlanOpen(true);
      onOpenChange(false);
      return;
    }
    if (status.custom_action?.action_kind === "assign_stand") {
      setStandOpen(true);
      onOpenChange(false);
      return;
    }
    if (status.custom_action?.action_kind === "runway_clearance") {
      runwayClearance(callsign);
      onOpenChange(false);
      return;
    }
    onOpenChange(false);
  }

  return (
    <Dialog
      open={validationDialogOpen}
      onOpenChange={(nextOpen) => {
        if (!nextOpen) {
          onOpenChange(false);
        }
      }}
    >
      <>
        <DialogContent className={dialogContentClassName} style={rootStyle}>
          <DialogTitle className="sr-only">Validation Status</DialogTitle>
          <div style={{ position: "relative", width: "100%", height: "100%" }}>
            <p className="sr-only">{status.issue_type}</p>

            <div
              style={{
                ...frameLineStyle,
                left: toVw(12),
                top: toVh(30),
                width: toVw(1),
                height: toVh(307),
              }}
            />
            <div
              style={{
                ...frameLineStyle,
                left: toVw(452),
                top: toVh(30),
                width: toVw(1),
                height: toVh(307),
              }}
            />
            <div
              style={{
                ...frameLineStyle,
                left: toVw(12),
                top: toVh(337),
                width: toVw(440),
                height: "1px",
              }}
            />
            <div
              style={{
                ...frameLineStyle,
                left: toVw(12),
                top: toVh(30),
                width: toVw(138.189),
                height: "1px",
              }}
            />
            <div
              style={{
                ...frameLineStyle,
                left: toVw(313.387),
                top: toVh(30),
                width: toVw(138.613),
                height: "1px",
              }}
            />

            <div
              style={{
                position: "absolute",
                top: toVh(15),
                left: "50%",
                transform: "translateX(-50%)",
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                gap: toVh(2),
                pointerEvents: "none",
              }}
            >
              <div
                style={{
                  fontSize: toVMin(16),
                  fontWeight: 400,
                  lineHeight: 1,
                  textAlign: "center",
                }}
              >
                VALIDATION STATUS
              </div>
              <div
                style={{
                  fontSize: toVMin(16),
                  fontWeight: 700,
                  lineHeight: 1,
                  textAlign: "center",
                }}
              >
                {callsign}
              </div>
            </div>

            <div style={messagePanelStyle}>
              <div style={getMessageTextStyle(status.message)}>
                {status.message}
              </div>
            </div>

            {status.custom_action ? (
              <button
                type="button"
                style={{
                  ...sharedButtonStyle,
                  left: toVw(129),
                  top: toVh(278),
                  width: toVw(103),
                  height: toVh(45),
                  flexDirection: "column",
                }}
                onClick={handleCustomAction}
                aria-label={status.custom_action.label}
              >
                {customActionLines.map((line) => (
                  <span key={line}>{line}</span>
                ))}
              </button>
            ) : (
              <div
                aria-hidden="true"
                style={{
                  ...sharedButtonStyle,
                  left: toVw(129),
                  top: toVh(278),
                  width: toVw(103),
                  height: toVh(45),
                }}
              />
            )}

            <button
              type="button"
              style={{
                ...sharedButtonStyle,
                left: toVw(288),
                top: toVh(267),
                width: toVw(155),
                height: toVh(29),
              }}
              onClick={handleAcknowledge}
            >
              ACKNOWLEDGE
            </button>
            <button
              type="button"
              style={{
                ...sharedButtonStyle,
                left: toVw(288),
                top: toVh(301),
                width: toVw(155),
                height: toVh(29),
              }}
              onClick={() => onOpenChange(false)}
            >
              ESC
            </button>
          </div>
        </DialogContent>
        <HoldingPointDialog
          open={holdingPointOpen}
          onOpenChange={(nextOpen) => {
            setHoldingPointOpen(nextOpen);
            if (!nextOpen) {
              onOpenChange(false);
            }
          }}
          callsign={callsign}
          runway={strip?.runway}
        />
        <FlightPlanDialog
          callsign={callsign}
          open={flightPlanOpen}
          onOpenChange={(nextOpen) => {
            setFlightPlanOpen(nextOpen);
            if (!nextOpen) {
              onOpenChange(false);
            }
          }}
          mode="clearance"
        />
        <ArrStandDialog
          open={standOpen}
          onOpenChange={(nextOpen) => {
            setStandOpen(nextOpen);
            if (!nextOpen) {
              onOpenChange(false);
            }
          }}
          callsign={callsign}
          currentStand={strip?.stand}
        />
      </>
    </Dialog>
  );
}
