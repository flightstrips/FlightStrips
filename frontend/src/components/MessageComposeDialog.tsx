import { useState, type CSSProperties, type ReactNode } from "react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { MENU_BUTTON_BEVEL, MENU_INPUT_BEVEL } from "@/components/ui/menuControlStyle";
import { EST_RAISED_EDGE } from "@/components/est/bevel";
import { RunwayDialog } from "@/components/strip/RunwayDialog";
import { TaxiwaySelectDialog } from "@/components/TaxiwaySelectDialog";
import { useWebSocketStore } from "@/store/store-hooks";
import { MESSAGE_MAX_CHARS } from "@/components/strip/MessageStrip";
import type { MessageFormat } from "@/api/models";
import { findStationController } from "@/lib/controllerStations";
import { scalePx } from "@/lib/viewportScale";

const PREDEFINED_MESSAGES = [
  "RUNWAY CHANGE TO 04R/04L", "RUNWAY CHANGE TO 22R/22L", "RUNWAY CHANGE TO 12/30",
  "CLOSING POSITION SOON", "PUSH AND HOLD AIRCRAFT +",
  "ATIS REPORTED DOWN. PLS PUT BACK ONLINE.", "ATIS REPORTING WRONG RUNWAY CONFIG",
  "ALL DEPARTURES - SA 3000FT", "ALL DEPARTURES - SA 4000FT", "SPECIFIC AIRCRAFT - SA 3000FT +",
];
const RECIPIENTS = [
  { label: "CLR-DEL", area: "CLR-DEL", frequency: "119.905", sectors: ["DEL"] },
  { label: "SEQ-PLN", area: "SEQ PLN", frequency: "121.905", sectors: ["SQ"] },
  { label: "APRON ARR", area: "APRON ARR", frequency: "121.630", sectors: ["AA"] },
  { label: "APRON DEP", area: "APRON DEP", frequency: "121.730", sectors: ["AD"] },
  { label: "GND WEST", area: "GND WEST", frequency: "118.580", sectors: ["GW"] },
  { label: "GND EAST", area: "GND EAST", frequency: "121.830", sectors: ["GE"] },
  { label: "TWR WEST", area: "TWR-ARR", frequency: "118.105", sectors: ["TW"] },
  { label: "TWR EAST", area: "TWR-DEP", frequency: "119.355", sectors: ["TE"] },
];
const frame = (left: number, top: number, width: number, height: number): CSSProperties => ({
  position: "absolute", left: scalePx(left), top: scalePx(top), width: scalePx(width), height: scalePx(height),
});
const inputStyle: CSSProperties = { ...MENU_INPUT_BEVEL, background: "#FCFCFC", color: "black", fontFamily: "var(--font-bay)", fontSize: scalePx(20), padding: scalePx(8), resize: "none" };
const validTime = (value: string) => /^([01]\d|2[0-3])[0-5]\d$/.test(value);

function Panel({ title, style, children }: { title: string; style: CSSProperties; children: ReactNode }) {
  return <fieldset style={{ ...style, border: "1px solid #666", padding: scalePx(10), margin: 0 }}>
    <legend style={{ margin: "auto", paddingInline: scalePx(5), fontSize: scalePx(14) }}>{title}</legend>
    {children}
  </fieldset>;
}

function MenuButton({ children, onClick, disabled = false, selected = false, style, label }: {
  children: ReactNode; onClick: () => void; disabled?: boolean; selected?: boolean; style?: CSSProperties; label?: string;
}) {
  return <button type="button" aria-label={label} disabled={disabled} aria-pressed={selected}
    className="flex items-center justify-center active:brightness-90"
    onClick={onClick} style={{ ...MENU_BUTTON_BEVEL, height: scalePx(35), fontFamily: "var(--font-bay)", fontSize: scalePx(14), fontWeight: 700,
      background: selected ? "#00FF26" : "#D6D6D6", color: disabled ? "#8A8A8A" : "black", cursor: disabled ? "default" : "pointer", ...style }}>{children}</button>;
}

function TimeKeypad({ kind, value, onChange }: { kind: "start" | "end"; value: string; onChange: (value: string) => void }) {
  const prefix = kind === "start" ? "Start time" : "End time";
  return <div style={{ width: scalePx(170), margin: "auto", paddingTop: scalePx(32) }}>
    <div className="flex justify-center" style={{ gap: scalePx(5), marginBottom: scalePx(5) }}>
      {kind === "start" ?
        <MenuButton label="Start time NOW" style={{ width: scalePx(112), height: scalePx(54) }} onClick={() => {
          const now = new Date();
          onChange(`${now.getUTCHours().toString().padStart(2, "0")}${now.getUTCMinutes().toString().padStart(2, "0")}`);
        }}>NOW</MenuButton> :
        <>
          <MenuButton label="End time ERASE" style={{ width: scalePx(112), height: scalePx(54) }} onClick={() => onChange("")}>ERASE</MenuButton>
          <MenuButton label="End time UFN" selected={value === "UFN"} style={{ width: scalePx(53), height: scalePx(54) }} onClick={() => onChange("UFN")}>UFN</MenuButton>
        </>}
    </div>
    <div className="grid grid-cols-3" style={{ gap: scalePx(5) }}>
      {["7", "8", "9", "4", "5", "6", "1", "2", "3"].map(digit =>
        <MenuButton key={digit} label={`${prefix} ${digit}`} style={{ height: scalePx(54) }} onClick={() => onChange((value === "UFN" || value.length === 4 ? "" : value) + digit)}>{digit}</MenuButton>)}
    </div>
    <MenuButton label={`${prefix} 0`} style={{ width: scalePx(53), height: scalePx(54), margin: `${scalePx(5)} auto` }}
      onClick={() => onChange((value === "UFN" || value.length === 4 ? "" : value) + "0")}>0</MenuButton>
    <MenuButton label={`${prefix} clear`} style={{ width: scalePx(112), height: scalePx(54), margin: "auto" }} onClick={() => onChange("")}>ERASE</MenuButton>
  </div>;
}

interface Props { open: boolean; onClose: () => void }

export function MessageComposeDialog({ open, onClose }: Props) {
  const sendMessage = useWebSocketStore(state => state.sendMessage);
  const controllers = useWebSocketStore(state => state.controllers);
  const ownPosition = useWebSocketStore(state => state.position);
  const ownCallsign = useWebSocketStore(state => state.callsign);
  const [text, setText] = useState("");
  const [freeText, setFreeText] = useState("");
  const [broadcast, setBroadcast] = useState(false);
  const [selectedAreas, setSelectedAreas] = useState<string[]>([]);
  const [startTime, setStartTime] = useState("");
  const [endTime, setEndTime] = useState("");
  const [format, setFormat] = useState<MessageFormat>("AUTO");
  const [taxiwayOpen, setTaxiwayOpen] = useState(false);
  const [runwayOpen, setRunwayOpen] = useState(false);
  const availableRecipients = RECIPIENTS.filter(station => findStationController(station, controllers, ownPosition, ownCallsign));
  const timing = [startTime ? `STARTING AT ${startTime}` : "", endTime ? `until ${endTime}` : ""].filter(Boolean).join(", ");
  const message = [text.trim(), freeText.trim(), timing].filter(Boolean).join(" ");
  const timeError = (startTime && !validTime(startTime)) || (endTime && endTime !== "UFN" && !validTime(endTime));
  const offlineRecipient = selectedAreas.some(area => !availableRecipients.some(station => station.area === area));
  const error = timeError ? "Enter complete UTC times in HHMM format (0000-2359)." :
    message.length > MESSAGE_MAX_CHARS ? `Message exceeds ${MESSAGE_MAX_CHARS} characters (${message.length}).` :
      offlineRecipient ? "A selected recipient is no longer online. Choose another recipient or BROADCAST." : "";

  function append(fragment: string) { setText(previous => [previous.trim(), fragment].filter(Boolean).join(" ")); }
  function resetMessage() { setText(""); setFreeText(""); setStartTime(""); setEndTime(""); }
  function eraseWord() {
    const removeLast = (value: string) => value.trimEnd().replace(/\S+\s*$/, "").trimEnd();
    if (freeText.trim()) setFreeText(removeLast);
    else setText(removeLast);
  }
  function send() {
    if (!message || error) return;
    sendMessage(message, selectedAreas, format);
    resetMessage();
    setSelectedAreas([]);
    setBroadcast(false);
    setFormat("AUTO");
    onClose();
  }

  return <>
    <Dialog open={open} onOpenChange={isOpen => { if (!isOpen) onClose(); }}>
      <DialogContent className="block max-w-none gap-0 p-0 [&>button:last-child]:hidden"
        style={{ ...EST_RAISED_EDGE, width: scalePx(1448), height: scalePx(768), background: "#B3B3B3", color: "black", fontFamily: "var(--font-bay)" }}>
        <DialogTitle style={{ ...frame(20, 12, 1400, 20), fontSize: scalePx(14) }}>MESSAGES</DialogTitle>
        <div className="pointer-events-none" style={{ ...frame(20, 34, 1402, 722), border: "1px solid #666" }} />
        <textarea aria-label="Message to send" readOnly value={message} placeholder="Message to send"
          style={{ ...inputStyle, ...frame(40, 43, 659, 47), textAlign: "center" }} />
        <input aria-label="Start time" readOnly value={startTime} style={{ ...inputStyle, ...frame(857, 48, 151, 37), textAlign: "center" }} />
        <span className="flex items-center justify-center" style={{ ...MENU_BUTTON_BEVEL, ...frame(1032, 48, 75, 37), background: "#FCFCFC", fontSize: scalePx(20) }}>UNTIL</span>
        <input aria-label="End time" readOnly value={endTime} style={{ ...inputStyle, ...frame(1131, 48, 151, 37), textAlign: "center" }} />

        <Panel title="RECIPIENTS" style={frame(43, 101, 279, 501)}>
          <MenuButton selected={broadcast} style={{ width: "100%", marginBottom: scalePx(18) }} onClick={() => { setBroadcast(true); setSelectedAreas([]); }}>BROADCAST</MenuButton>
          <div className="grid grid-cols-2" style={{ gap: scalePx(16), marginBottom: scalePx(16) }}>
            {RECIPIENTS.map(station => <MenuButton key={station.area} selected={selectedAreas.includes(station.area)}
              disabled={!availableRecipients.includes(station)} onClick={() => {
                setBroadcast(false);
                setSelectedAreas(previous => previous.includes(station.area) ? previous.filter(area => area !== station.area) : [...previous, station.area]);
              }}>{station.label}</MenuButton>)}
          </div>
          <div className="flex flex-col" style={{ gap: scalePx(4) }}>
            <MenuButton onClick={() => setTaxiwayOpen(true)} style={{ background: "#959595" }}>TWY</MenuButton>
            <MenuButton onClick={() => setRunwayOpen(true)} style={{ background: "#959595" }}>RWY</MenuButton>
            <MenuButton onClick={() => append("Closed")}>+ CLOSED</MenuButton>
            <MenuButton onClick={() => append("Out of use")}>+ OUT OF USE</MenuButton>
            <MenuButton label="Erase last word" style={{ marginTop: scalePx(12) }} onClick={eraseWord}>ERASE</MenuButton>
          </div>
        </Panel>
        <MenuButton style={{ ...frame(54, 614, 254, 33), background: "#3F3F3F", color: "white", outline: "1px solid #FF0000" }} onClick={resetMessage}>DELETE MESSAGE</MenuButton>
        <MenuButton style={{ ...frame(54, 696, 118, 42), background: "#3F3F3F", color: "white", fontSize: scalePx(20) }} onClick={onClose}>ESC</MenuButton>
        <MenuButton disabled={!message || !!error} style={{ ...frame(193, 696, 118, 42), background: "#3F3F3F", color: !message || error ? "#8A8A8A" : "white", fontSize: scalePx(20) }} onClick={send}>OK</MenuButton>

        <Panel title="PRE-DEFINED MESSAGES" style={frame(335, 101, 384, 466)}>
          <div className="flex flex-col" style={{ gap: scalePx(4), paddingInline: scalePx(4) }}>
            {PREDEFINED_MESSAGES.map((preset, index) => <MenuButton key={preset} onClick={() => append(preset)}
              style={{ justifyContent: "flex-start", textAlign: "left", paddingInline: scalePx(2), height: scalePx(33), fontWeight: 400, marginTop: [3, 5, 7].includes(index) ? scalePx(17) : 0 }}>{preset}</MenuButton>)}
          </div>
        </Panel>
        <Panel title="FORMAT" style={frame(335, 579, 384, 139)}>
          <div className="grid grid-cols-3" style={{ gap: scalePx(5), paddingTop: scalePx(8) }}>
            {(["SMALL", "AUTO", "LARGE"] as const).map(option => <MenuButton key={option} selected={format === option} onClick={() => setFormat(option)}>{option}</MenuButton>)}
          </div>
        </Panel>
        <Panel title="START TIME" style={frame(745, 101, 302, 419)}><TimeKeypad kind="start" value={startTime} onChange={setStartTime} /></Panel>
        <Panel title="END TIME" style={frame(1093, 101, 302, 419)}><TimeKeypad kind="end" value={endTime} onChange={setEndTime} /></Panel>
        <Panel title="ADDITIONAL FREE TEXT MESSAGES" style={frame(745, 552, 650, 166)}>
          <textarea aria-label="Additional free text" value={freeText} maxLength={MESSAGE_MAX_CHARS}
            onChange={event => setFreeText(event.target.value)} placeholder="Text can be written down here"
            style={{ ...inputStyle, width: "100%", height: "100%", textAlign: "center" }} />
        </Panel>
        {error && <div role="alert" style={{ ...frame(335, 728, 1060, 23), color: "#8B0000", fontSize: scalePx(14) }}>{error}</div>}
      </DialogContent>
    </Dialog>
    {taxiwayOpen && <TaxiwaySelectDialog onClose={() => setTaxiwayOpen(false)} onSelect={taxiway => append(`TWY ${taxiway}`)} />}
    {runwayOpen && <RunwayDialog open mode="SELECT" onOpenChange={setRunwayOpen} onSelect={runway => append(`RWY ${runway}`)} />}
  </>;
}
