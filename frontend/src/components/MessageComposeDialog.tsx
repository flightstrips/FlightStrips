import { useState } from "react";
import { Dialog, DialogContent } from "@/components/ui/dialog.tsx";
import { useWebSocketStore } from "@/store/store-hooks.ts";
import { MESSAGE_MAX_CHARS } from "@/components/strip/MessageStrip.tsx";

const PREDEFINED_MESSAGES = [
  "RUNWAY CHANGE TO 04R/04L",
  "RUNWAY CHANGE TO 22R/22L",
  "RUNWAY CHANGE TO 12/30",
  "CLOSING POSITION SOON",
  "ENFORCE A-CDM! TRAFFIC LOAD TOO HIGH",
  "ALL DEPARTURES MUST BE CLRD RWY-HDG TO 4000' UFN",
  "ATIS REPORTED DOWN. PLS PUT BACK ONLINE.",
  "ATIS REPORTING WRONG RUNWAY CONFIG",
];

const AREA_PAIRS: [string, string][] = [
  ["CLR-DEL", "SEQ PLN"],
  ["APRON ARR", "APRON DEP"],
  ["GND WEST", "GND EAST"],
  ["TWR-ARR", "TWR-DEP"],
];

const ALWAYS_RED = new Set(["GND EAST"]);

const DROP_SHADOW = "0 4px 4px rgba(0,0,0,0.25)";

const BTN: React.CSSProperties = {
  fontFamily: "Rubik, sans-serif",
  fontWeight: 600,
  fontSize: 24,
  border: "none",
  cursor: "pointer",
  color: "#000",
  boxShadow: DROP_SHADOW,
};

interface Props {
  open: boolean;
  onClose: () => void;
}

export function MessageComposeDialog({ open, onClose }: Props) {
  const sendMessage = useWebSocketStore(s => s.sendMessage);
  const [text, setText] = useState("");
  const [broadcastSelected, setBroadcastSelected] = useState(false);
  const [selectedAreas, setSelectedAreas] = useState<Set<string>>(new Set());
  const [selectedPredefined, setSelectedPredefined] = useState<number | null>(null);

  function handleBroadcast() {
    setBroadcastSelected(true);
    setSelectedAreas(new Set());
  }

  function toggleArea(area: string) {
    if (ALWAYS_RED.has(area)) return;
    setBroadcastSelected(false);
    setSelectedAreas(prev => {
      const next = new Set(prev);
      if (next.has(area)) next.delete(area);
      else next.add(area);
      return next;
    });
  }

  function handlePredefined(i: number) {
    setSelectedPredefined(i);
    setText(PREDEFINED_MESSAGES[i]);
  }

  function handleErase() {
    setText("");
    setSelectedPredefined(null);
  }

  function handleOk() {
    if (!text.trim()) return;
    sendMessage(text.trim(), Array.from(selectedAreas));
    setText("");
    setBroadcastSelected(false);
    setSelectedAreas(new Set());
    setSelectedPredefined(null);
    onClose();
  }

  function handleOpenChange(isOpen: boolean) {
    if (!isOpen) onClose();
  }

  function areaBg(area: string) {
    if (ALWAYS_RED.has(area)) return "#FF6D4D";
    if (selectedAreas.has(area)) return "#70ED45";
    return "#D6D6D6";
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        style={{
          background: "#E4E4E4",
          border: "1px solid black",
          color: "#000",
          // SVG canvas is 1291×824; inner box starts at x=51,y=42
          width: 1291,
          maxWidth: "95vw",
          padding: "20px 51px 20px",
          display: "flex",
          flexDirection: "column",
          gap: 0,
        }}
      >
        {/* FREE TEXT — font-size 24, weight 300, centered */}
        <div style={{
          textAlign: "center",
          fontFamily: "Rubik, sans-serif",
          fontSize: 24,
          fontWeight: 300,
          marginBottom: 16,
        }}>
          FREE TEXT
        </div>

        {/* Text input — #FCFCFC, height 113px matching SVG */}
        <textarea
          style={{
            width: "100%",
            height: 113,
            background: "#FCFCFC",
            border: "1px solid black",
            fontFamily: "Rubik, sans-serif",
            fontSize: 24,
            padding: "8px 12px",
            resize: "none",
            boxSizing: "border-box",
            boxShadow: DROP_SHADOW,
            marginBottom: 28,
          }}
          maxLength={MESSAGE_MAX_CHARS}
          value={text}
          onChange={e => { setText(e.target.value); setSelectedPredefined(null); }}
          placeholder="Text can be written down here"
        />

        {/* Main content row: left panel (362px) + 55px gap + right column (707px) */}
        <div style={{ display: "flex", gap: 55, alignItems: "flex-start" }}>

          {/* Left panel — #B3B3B3, 362px wide, padded to match SVG button positions */}
          <div style={{
            width: 362,
            flexShrink: 0,
            background: "#B3B3B3",
            border: "1px solid black",
            // inner padding: top=29, left=34, right=22, bottom=19
            padding: "29px 22px 19px 34px",
            display: "flex",
            flexDirection: "column",
            boxSizing: "border-box",
          }}>
            {/* BROADCAST — full inner width (362-34-22=306px), height 55px */}
            <button
              style={{
                ...BTN,
                width: "100%",
                height: 55,
                background: broadcastSelected ? "#70ED45" : "#D6D6D6",
              }}
              onClick={handleBroadcast}
            >
              BROADCAST
            </button>

            {/* 25px gap below BROADCAST (larger than between rows) */}
            <div style={{ height: 25 }} />

            {/* 2-column area grid: 146px columns, 14px gap */}
            <div style={{
              display: "grid",
              gridTemplateColumns: "146px 146px",
              gap: 14,
            }}>
              {AREA_PAIRS.flatMap(([left, right]) => [
                <button
                  key={left}
                  style={{ ...BTN, height: 55, background: areaBg(left) }}
                  onClick={() => toggleArea(left)}
                >
                  {left}
                </button>,
                <button
                  key={right}
                  style={{ ...BTN, height: 55, background: areaBg(right) }}
                  onClick={() => toggleArea(right)}
                >
                  {right}
                </button>,
              ])}
            </div>
          </div>

          {/* Right column: 707px — messages then buttons */}
          <div style={{ width: 707, flexShrink: 0 }}>
            {/* 8 predefined messages: height 42px, gap 8px → 392px total */}
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {PREDEFINED_MESSAGES.map((msg, i) => (
                <button
                  key={i}
                  style={{
                    textAlign: "left",
                    padding: "0 12px",
                    fontFamily: "Rubik, sans-serif",
                    fontSize: 24,
                    fontWeight: 400,
                    border: "none",
                    cursor: "pointer",
                    height: 42,
                    background: selectedPredefined === i ? "#1BFF16" : "#D6D6D6",
                    color: "#000",
                    boxShadow: DROP_SHADOW,
                  }}
                  onClick={() => handlePredefined(i)}
                >
                  {msg}
                </button>
              ))}
            </div>

            {/* 11px gap then ERASE (200px) + 37px gap + OK (200px) */}
            <div style={{ display: "flex", gap: 37, marginTop: 11 }}>
              <button
                style={{
                  width: 200,
                  height: 70,
                  background: "#3F3F3F",
                  color: "#fff",
                  fontFamily: "Rubik, sans-serif",
                  fontWeight: 600,
                  fontSize: 32,
                  border: "none",
                  cursor: "pointer",
                  boxShadow: DROP_SHADOW,
                }}
                onClick={handleErase}
              >
                ERASE
              </button>
              <button
                style={{
                  width: 200,
                  height: 70,
                  background: "#3F3F3F",
                  color: "#fff",
                  fontFamily: "Rubik, sans-serif",
                  fontWeight: 600,
                  fontSize: 32,
                  border: "none",
                  cursor: "pointer",
                  boxShadow: DROP_SHADOW,
                }}
                onClick={handleOk}
              >
                OK
              </button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
