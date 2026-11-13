import type { MessageReceived } from "@/api/models.ts";
import { useWebSocketStore } from "@/store/store-hooks.ts";
import { COLOR_BTN_ORANGE, COLOR_SI_ASSUMED, COLOR_SI_CONCERNED, getFramedStripStyle } from "@/components/strip/shared";

export const MESSAGE_MAX_CHARS = 120;

// Message strip color constants
const COLOR_STRIP_BG       = "#285A5C"; // teal background for message strips
const COLOR_TEXT_MAIN      = "#E9E9E9"; // primary text / button border color
const COLOR_TEXT_ON_LIGHT  = "#1a1a1a"; // dark text when SI background is light

function getMessageSI(msg: MessageReceived, currentPosition: string): { background: string; color: string; initials: string; canAcknowledge: boolean } {
  if (msg.sender === "SYSTEM") return { background: COLOR_SI_CONCERNED, color: "white", initials: "SY", canAcknowledge: false };

  const isSingleControllerMessage = !msg.is_broadcast && msg.recipients.length === 1 && (msg.receipt_positions?.length ?? 0) > 0;
  if (!isSingleControllerMessage) {
    return { background: COLOR_BTN_ORANGE, color: "white", initials: "", canAcknowledge: false };
  }

  const raw = msg.sender.replace(/[^A-Z0-9]/gi, "");
  const initials = raw.slice(-2).toUpperCase();
  const background = msg.acknowledged
    ? COLOR_BTN_ORANGE
    : `linear-gradient(to right, ${COLOR_SI_ASSUMED} 50%, ${COLOR_BTN_ORANGE} 50%)`;
  return {
    background,
    color: msg.acknowledged ? "white" : COLOR_TEXT_ON_LIGHT,
    initials,
    canAcknowledge: !msg.acknowledged && msg.receipt_positions?.includes(currentPosition) === true,
  };
}

interface MessageStripProps {
  msg: MessageReceived;
}

export function MessageStrip({ msg }: MessageStripProps) {
  const position = useWebSocketStore(s => s.position);
  const acknowledgeMessage = useWebSocketStore(s => s.acknowledgeMessage);
  const dismissMessage = useWebSocketStore(s => s.dismissMessage);
  const si = getMessageSI(msg, position);
  const largeText = msg.format === "LARGE" ||
    ((!msg.format || msg.format === "AUTO") && !msg.is_broadcast && msg.recipients.length === 1);

  return (
    <div
      className="flex items-stretch shrink-0"
      style={{ minHeight: "2.83dvh", width: "95%", ...getFramedStripStyle(false, COLOR_STRIP_BG) }}
    >
      {/* SI box */}
      <div
        role={si.canAcknowledge ? "button" : undefined}
        tabIndex={si.canAcknowledge ? 0 : undefined}
        aria-label={si.canAcknowledge ? "Acknowledge message receipt" : undefined}
        className={`flex items-center justify-center shrink-0 font-normal [-webkit-text-stroke:0.5px_currentColor]${si.canAcknowledge ? " cursor-pointer" : ""}`}
        style={{ width: "1.88vw", background: si.background, color: si.color, fontSize: "0.73vw" }}
        onClick={si.canAcknowledge ? () => acknowledgeMessage(msg.id) : undefined}
        onKeyDown={si.canAcknowledge ? event => {
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            acknowledgeMessage(msg.id);
          }
        } : undefined}
        title={si.canAcknowledge ? "Acknowledge receipt" : undefined}
      >
        {si.initials}
      </div>

      {/* Message text */}
      <div
        className="flex-1 flex items-center px-[0.42vw] py-[0.3dvh]"
        style={{ fontFamily: "var(--font-bay)", fontSize: largeText ? "1.04vw" : "0.73vw", color: COLOR_TEXT_MAIN }}
      >
        {/* ph-no-capture: keep private message text out of analytics and session replay */}
        <span className="ph-no-capture break-words min-w-0 w-full">{msg.text}</span>
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
