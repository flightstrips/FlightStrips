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
          // 1300px ≈ SVG 1291px; scales down to 95vw on smaller screens
          width: "min(1300px, 95vw)",
          padding: "20px 51px",
          display: "flex",
          flexDirection: "column",
          gap: 0,
        }}
      >
        {/* FREE TEXT — centered title */}
        <div style={{
          textAlign: "center",
          fontFamily: "Rubik, sans-serif",
          fontSize: 24,
          fontWeight: 300,
          marginBottom: 16,
        }}>
          FREE TEXT
        </div>

        {/* Text input — width:100% fills the same content area as the main row below */}
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

        {/* Main row: left (flex 362) + fixed 55px gap + right (flex 707) */}
        <div style={{
          display: "flex",
          gap: 55,
          alignItems: "flex-start",
        }}>

            {/* Left panel — #B3B3B3, proportional width (362 parts of 1069) */}
            <div style={{
              flex: "362 0 0",
              minWidth: 0,
              background: "#B3B3B3",
              border: "1px solid black",
              // padding scaled to match SVG: top=29, left=34, right=22, bottom=19
              padding: "29px 22px 19px 34px",
              display: "flex",
              flexDirection: "column",
              boxSizing: "border-box",
            }}>
              {/* BROADCAST — full inner width, height 55px */}
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

              {/* 25px gap below BROADCAST — larger than between area rows */}
              <div style={{ height: 25 }} />

              {/* 2-column area grid, equal columns, 14px gap */}
              <div style={{
                display: "grid",
                gridTemplateColumns: "1fr 1fr",
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

            {/* Right column: proportional width (707 parts of 1069) */}
            <div style={{ flex: "707 0 0", minWidth: 0 }}>
              {/* 8 messages: 42px height, 8px gap → all visible, no scroll */}
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

              {/* ERASE + OK: 200px each, 37px apart, 11px below messages */}
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
