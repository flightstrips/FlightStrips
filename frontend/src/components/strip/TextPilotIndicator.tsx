import { CommunicationType } from "@/api/models";

/** The square scales with its cell, including compact and full-width strips. */
export function TextPilotIndicator({ type, borderColor = "currentColor", divider = true }: { type?: CommunicationType; borderColor?: string; divider?: boolean }) {
  const text = type === CommunicationType.Text;
  const receive = type === CommunicationType.Receive;
  return <span aria-label={text ? "Text-only pilot" : receive ? "Receive-only pilot" : "Voice pilot"}
    data-testid="text-pilot-indicator" data-communication-type={type ?? ""}
    style={{ position: "absolute", bottom: 0, right: 0, height: "33.333333%", aspectRatio: "1", borderLeft: divider ? `1px solid ${borderColor}` : undefined,
      backgroundColor: text ? "#FF6D4D" : undefined, color: "#2F2F2F", display: "flex", alignItems: "center", justifyContent: "center",
      fontFamily: "Rubik, sans-serif", fontWeight: 400, containerType: "size", lineHeight: 1, pointerEvents: "none" }}>
    <span style={{ fontSize: "80cqh" }}>{text ? "t" : receive ? "r" : ""}</span>
  </span>;
}
