import type { MessageReceived } from "@/api/models.ts";
import { useWebSocketStore } from "@/store/store-hooks.ts";

export const MESSAGE_MAX_CHARS = 120;

function getMessageSI(msg: MessageReceived, currentPosition: string): { color: string; initials: string } {
  if (msg.sender === "SYSTEM") return { color: "#E082E7", initials: "SY" };

  const isSentByMe = msg.sender === currentPosition;

  if (isSentByMe && (msg.is_broadcast || msg.recipients.length > 1)) {
    return { color: "#FF6D4D", initials: "" };
  }

  if (!isSentByMe && msg.is_broadcast) {
    return { color: "#E082E7", initials: "" };
  }

  // Personal message — show sender's last 2 chars as initials
  const raw = msg.sender.replace(/[^A-Z0-9]/gi, "");
  const initials = raw.slice(-2).toUpperCase();
  return { color: "#F0F0F0", initials };
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
      style={{ height: 48, background: "#285A5C" }}
    >
      {/* SI box */}
      <div
        className="flex items-center justify-center shrink-0 font-bold text-sm"
        style={{ width: 36, background: si.color, color: si.color === "#F0F0F0" ? "#1a1a1a" : "#fff" }}
      >
        {si.initials}
      </div>

      {/* Message text */}
      <div
        className="flex-1 flex items-center px-2 overflow-hidden"
        style={{ fontFamily: "Rubik, sans-serif", fontSize: 14, color: "#E9E9E9" }}
      >
        <span className="truncate">{msg.text}</span>
      </div>

      {/* X button */}
      <button
        className="flex items-center justify-center shrink-0 border border-[#E9E9E9] m-1 font-bold text-[#E9E9E9] hover:bg-[#1e4547] active:bg-[#163638]"
        style={{ width: 30, height: 30 }}
        onClick={() => dismissMessage(msg.id)}
        title="Dismiss"
      >
        X
      </button>
    </div>
  );
}
