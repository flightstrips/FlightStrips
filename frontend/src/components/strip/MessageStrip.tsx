import type { MessageReceived } from "@/api/models.ts";
import { useWebSocketStore } from "@/store/store-hooks.ts";
import { COLOR_SI_ASSUMED, COLOR_SI_CONCERNED, getFramedStripStyle } from "@/components/strip/shared";

export const MESSAGE_MAX_CHARS = 120;

// Message strip color constants
const COLOR_STRIP_BG       = "#285A5C"; // teal background for message strips
const COLOR_SI_BROADCAST   = "#FF6D4D"; // sent-by-me broadcast indicator
const COLOR_TEXT_MAIN      = "#E9E9E9"; // primary text / button border color
const COLOR_TEXT_ON_LIGHT  = "#1a1a1a"; // dark text when SI background is light

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
      style={{ minHeight: "2.83dvh", width: "95%", ...getFramedStripStyle(false, COLOR_STRIP_BG) }}
    >
      {/* SI box */}
      <div
        className="flex items-center justify-center shrink-0 font-normal [-webkit-text-stroke:0.5px_currentColor]"
        style={{ width: "1.88vw", background: si.color, color: si.color === COLOR_SI_ASSUMED ? COLOR_TEXT_ON_LIGHT : "white", fontSize: "0.73vw" }}
      >
        {si.initials}
      </div>

      {/* Message text */}
      <div
        className="flex-1 flex items-center px-[0.42vw] py-[0.3dvh]"
        style={{ fontFamily: "var(--font-bay)", fontSize: "0.73vw", color: COLOR_TEXT_MAIN }}
      >
        <span className="break-words min-w-0 w-full">{msg.text}</span>
      </div>

      {/* X button — same square-and-cross as the tactical strips */}
      <div
        className="flex-shrink-0 flex items-center justify-center cursor-pointer"
        style={{ width: "1.25vw", color: COLOR_TEXT_MAIN }}
        onClick={() => dismissMessage(msg.id)}
        title="Dismiss"
      >
        <span style={{ fontFamily: "var(--font-bay)", fontSize: "1.1dvh", lineHeight: 1, width: "1.7dvh", height: "1.7dvh", flexShrink: 0, display: "flex", alignItems: "center", justifyContent: "center", border: "1px solid currentColor", borderRadius: "0.4dvh", boxSizing: "border-box" }}>✕</span>
      </div>
    </div>
  );
}
