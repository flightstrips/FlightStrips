import type { MessageReceived } from "@/api/models.ts";
import { useWebSocketStore } from "@/store/store-hooks.ts";
import { COLOR_SI_ASSUMED, COLOR_SI_CONCERNED } from "@/components/strip/shared";

export const MESSAGE_MAX_CHARS = 120;

// Message strip color constants
const COLOR_STRIP_BG       = "#285A5C"; // teal background for message strips
const COLOR_SI_BROADCAST   = "#FF6D4D"; // sent-by-me broadcast indicator
const COLOR_TEXT_MAIN      = "#E9E9E9"; // primary text / button border color
const COLOR_TEXT_ON_LIGHT  = "#1a1a1a"; // dark text when SI background is light
// Tailwind class constants (hex must be literal strings for JIT)
const CLS_DISMISS_BTN = "flex items-center justify-center shrink-0 border border-[#E9E9E9] m-1 font-bold text-[#E9E9E9] hover:bg-[#1e4547] active:bg-[#163638]";

function getMessageSI(msg: MessageReceived, currentPosition: string): { color: string; initials: string } {
  if (msg.sender === "SYSTEM") return { color: COLOR_SI_CONCERNED, initials: "SY" };

  const isSentByMe = msg.sender === currentPosition;

  if (isSentByMe && (msg.is_broadcast || msg.recipients.length > 1)) {
    return { color: COLOR_SI_BROADCAST, initials: "" };
  }

  if (!isSentByMe && msg.is_broadcast) {
    return { color: COLOR_SI_CONCERNED, initials: "" };
  }

  // Personal message — show sender's last 2 chars as initials
  const raw = msg.sender.replace(/[^A-Z0-9]/gi, "");
  const initials = raw.slice(-2).toUpperCase();
  return { color: COLOR_SI_ASSUMED, initials };
}

interface MessageStripProps {
  msg: MessageReceived;
}

export function MessageStrip({ msg }: MessageStripProps) {
  const position = useWebSocketStore(s => s.position);
  const dismissMessage = useWebSocketStore(s => s.dismissMessage);
  const si = getMessageSI(msg, position);

  return (
    <div
      className="flex items-stretch shrink-0"
      style={{ minHeight: "4.72vh", background: COLOR_STRIP_BG }}
    >
      {/* SI box */}
      <div
        className="flex items-center justify-center shrink-0 font-bold text-sm"
        style={{ width: 36, background: si.color, color: si.color === COLOR_SI_ASSUMED ? COLOR_TEXT_ON_LIGHT : "white" }}
      >
        {si.initials}
      </div>

      {/* Message text */}
      <div
        className="flex-1 flex items-center px-2 py-2"
        style={{ fontFamily: "Rubik, sans-serif", fontSize: 14, color: COLOR_TEXT_MAIN }}
      >
        <span className="break-words min-w-0 w-full">{msg.text}</span>
      </div>

      {/* X button */}
      <button
        className={CLS_DISMISS_BTN}
        style={{ width: 30, height: 30 }}
        onClick={() => dismissMessage(msg.id)}
        title="Dismiss"
      >
        X
      </button>
    </div>
  );
}
