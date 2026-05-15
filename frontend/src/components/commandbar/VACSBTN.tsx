import { useCallback, useState } from "react";
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
import { Button } from "@/components/ui/button";

const BTN_BASE =
  "relative h-[3.42dvh] my-[0.65dvh] w-[3.52vw] flex items-center justify-center shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow)] outline-none";

function tooltipForState(state: VacsState): string {
  switch (state.status) {
    case "unavailable":
      return "VACS not running, or remote control not enabled in VACS settings.";
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
    case "connected": {
      const name = state.peer?.displayName ?? "controller";
      return `On call with ${name}. Click for options.`;
    }
  }
}

function buttonClass(state: VacsState): string {
  switch (state.status) {
    case "idle":
      return `${BTN_BASE} bg-bay-btn text-white`;
    case "incoming":
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
  const [popoverOpen, setPopoverOpen] = useState(false);

  const disabled =
    state.status === "unavailable" ||
    state.status === "unauthenticated" ||
    state.status === "disconnected" ||
    state.status === "ambiguous";

  const incomingCount = state.status === "incoming" ? state.calls.length : 0;

  const handleClick = useCallback(async () => {
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
    if (state.status === "connected") {
      setPopoverOpen((v) => !v);
    }
  }, [actions, state]);

  const handleContextMenu = useCallback(
    async (e: React.MouseEvent) => {
      e.preventDefault();
      if (state.status === "incoming") {
        const oldest = state.calls[0];
        if (oldest) {
          await actions.rejectCall(oldest.callId);
        }
        return;
      }
      if (state.status === "connected") {
        await actions.endCall(state.callId);
        setPopoverOpen(false);
      }
    },
    [actions, state],
  );

  const handleEndCall = useCallback(async () => {
    if (state.status === "connected") {
      await actions.endCall(state.callId);
      setPopoverOpen(false);
    }
  }, [actions, state]);

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
            className={buttonClass(state)}
            onClick={() => void handleClick()}
            onContextMenu={(e) => void handleContextMenu(e)}
            aria-label="VACS voice"
          >
            <Phone className="w-[1.6vw] h-[1.6vw]" />
            {incomingCount > 1 && (
              <span className="absolute -top-1 -right-1 min-w-[1.1em] h-[1.1em] px-0.5 rounded-full bg-black text-white text-[0.55vw] font-bold flex items-center justify-center">
                {incomingCount}
              </span>
            )}
          </button>
        </TooltipTrigger>
        <TooltipContent side="top" className="max-w-xs">
          {tooltipForState(state)}
        </TooltipContent>
      </Tooltip>

      {state.status === "idle" && (
        <VacsDialModal
          open={dialOpen}
          onOpenChange={setDialOpen}
          clients={state.clients}
          ambiguous={false}
        />
      )}

      {state.status === "connected" && popoverOpen && (
        <>
          <div
            className="fixed inset-0 z-[100]"
            onClick={() => setPopoverOpen(false)}
            aria-hidden
          />
          <div className="absolute bottom-[5.5dvh] right-[8vw] z-[101] bg-[#b3b3b3] border-2 border-black p-3 min-w-[200px] shadow-lg">
            <p className="text-sm font-semibold text-black mb-2">
              {state.peer?.displayName ?? "On call"}
            </p>
            {state.peer?.frequency && (
              <p className="text-xs text-gray-700 mb-2">{state.peer.frequency}</p>
            )}
            <Button variant="darkaction" className="w-full" onClick={() => void handleEndCall()}>
              End call
            </Button>
          </div>
        </>
      )}
    </>
  );
}
