import { useCallback, useEffect, useRef, useState } from "react";
import { Phone } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useVacs } from "@/hooks/useVacs";
import { useVacsSettings } from "@/hooks/useVacsSettings";
import type { VacsState } from "@/vacs/types";
import VacsDialModal from "./VacsDialModal";
import { CLS_CMD_BEVEL } from "@/components/strip/shared";

const BTN_BASE =
  `relative h-[3.42dvh] my-[0.65dvh] w-[3.52vw] flex items-center justify-center ${CLS_CMD_BEVEL} outline-none`;

const ENDING_FLASH_MS = 700;

function tooltipForState(state: VacsState, endingCall: boolean): string {
  if (endingCall) {
    return "Ending call…";
  }
  if (state.tooltip) return state.tooltip;
  switch (state.status) {
    case "disabled": return "Switch VACS On in the FlightStrips EuroScope panel.";
    case "reconnecting": return "The EuroScope plugin is reconnecting to VACS.";
    case "synchronizing": return "Restoring VACS call state. Controls will be available when synchronized.";
    case "unavailable": return "Connect your EuroScope plugin, start VACS and enable remote control.";
    case "unauthenticated":
      return "Sign in to VATSIM in VACS to enable voice.";
    case "disconnected":
      return "VACS is not connected to a signaling position.";
    case "ambiguous":
      return "Your VACS position is ambiguous — resolve it in VACS.";
    case "idle":
      return "Voice — click to dial a controller.";
    case "incoming": {
      const first = state.calls[0];
      const pos = first?.source.positionId ?? first?.source.clientId ?? "unknown";
      return `Incoming call from ${pos}. Click to accept, right-click to reject.`;
    }
    case "outgoing": {
      const name = state.peer.displayName ?? state.peer.positionId ?? "controller";
      return `Calling ${name}…`;
    }
    case "connecting":
    case "connected": {
      const name = state.peer?.displayName ?? "controller";
      return `On call with ${name}. Click to end call.`;
    }
  }
}

function buttonClass(state: VacsState, endingCall: boolean): string {
  if (endingCall) {
    return `${BTN_BASE} bg-[#FF4444] text-white`;
  }
  switch (state.status) {
    case "idle":
      return `${BTN_BASE} bg-bay-btn text-white`;
    case "incoming":
    case "outgoing":
    case "connecting":
      return `${BTN_BASE} bg-[#FF8C00] text-white animate-vacs-pulse`;
    case "connected":
      return `${BTN_BASE} bg-[#1BFF16] text-black`;
    default:
      return `${BTN_BASE} bg-bay-btn text-white opacity-50 cursor-not-allowed`;
  }
}

export default function VACSBTN() {
  const { vacsEnabled } = useVacsSettings();
  const { state, actions } = useVacs();
  const [dialOpen, setDialOpen] = useState(false);
  const [endingCall, setEndingCall] = useState(false);
  const endingTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (endingTimerRef.current !== null) {
        clearTimeout(endingTimerRef.current);
      }
    };
  }, []);

  const flashEnding = useCallback(() => {
    setEndingCall(true);
    if (endingTimerRef.current !== null) {
      clearTimeout(endingTimerRef.current);
    }
    endingTimerRef.current = setTimeout(() => {
      setEndingCall(false);
      endingTimerRef.current = null;
    }, ENDING_FLASH_MS);
  }, []);

  const endActiveCall = useCallback(async () => {
    if (state.status !== "connected" && state.status !== "connecting") {
      return;
    }
    flashEnding();
    await actions.endCall(state.callId);
  }, [actions, state, flashEnding]);

  const disabled = state.actionable === false || !["idle", "incoming", "outgoing", "connecting", "connected"].includes(state.status);
  const incomingCount = state.status === "incoming" ? state.waiting ?? state.calls.length : 0;
  const isEndingCall = endingCall && (state.status === "connected" || state.status === "connecting");

  const handleClick = useCallback(async () => {
    if (disabled) return;
    if (state.status === "idle") {
      setDialOpen(true);
      return;
    }
    if (state.status === "incoming") {
      const oldest = state.calls[0];
      if (oldest) {
        await actions.acceptCall(oldest.callId);
      }
      return;
    }
    if (state.status === "outgoing") {
      await actions.endCall(state.callId);
      return;
    }
    if (state.status === "connected" || state.status === "connecting") {
      await endActiveCall();
    }
  }, [actions, state, endActiveCall, disabled]);

  const handleContextMenu = useCallback(
    async (e: React.MouseEvent) => {
      e.preventDefault();
      if (disabled) return;
      if (state.status === "incoming") {
        const oldest = state.calls[0];
        if (oldest) {
          await actions.rejectCall(oldest.callId);
        }
        return;
      }
      if (state.status === "outgoing") {
        await actions.endCall(state.callId);
        return;
      }
      if (state.status === "connected" || state.status === "connecting") {
        await endActiveCall();
      }
    },
    [actions, state, endActiveCall, disabled],
  );

  if (!vacsEnabled) {
    return null;
  }

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            disabled={disabled}
            className={buttonClass(state, isEndingCall)}
            onClick={() => void handleClick().catch(() => undefined)}
            onContextMenu={(e) => void handleContextMenu(e).catch(() => undefined)}
            aria-label="VACS voice"
          >
            <Phone className="w-[1.6vw] h-[1.6vw]" />
            {incomingCount > 1 && (
              <span className="absolute -top-1 -right-1 min-w-[1.1em] h-[1.1em] px-0.5 rounded-full bg-black text-white text-[0.4125vw] font-bold flex items-center justify-center">
                {incomingCount}
              </span>
            )}
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs">
          {tooltipForState(state, isEndingCall)}
        </TooltipContent>
      </Tooltip>

      {state.status === "idle" && (
        <VacsDialModal
          open={dialOpen}
          onOpenChange={setDialOpen}
          clients={state.clients}
          ownClientId={state.ownClientId}
          ownPositionId={state.ownPositionId}
          ambiguous={false}
          actionable={state.actionable ?? true}
        />
      )}
    </>
  );
}
