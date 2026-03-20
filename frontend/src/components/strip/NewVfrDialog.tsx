import { useState, useEffect } from "react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { useStrips, useWebSocketStore, useMetar } from "@/store/store-hooks";
import { decodeMetar } from "@/lib/metarDecode";

const DROP_SHADOW = "0 4px 4px rgba(0,0,0,0.25)";
const BG = "#D5D5D5";
const BTN_GRAY = "#9E989C";
const BTN_DARK = "#3F3F3F";
const INPUT_BG = "#EDEDED";
const FONT = "'Rubik', sans-serif";

function btnGray(width: number, height = 26): React.CSSProperties {
  return {
    width,
    height,
    background: BTN_GRAY,
    border: "none",
    fontSize: 14,
    fontWeight: 600,
    fontFamily: FONT,
    cursor: "pointer",
    boxShadow: DROP_SHADOW,
    flexShrink: 0,
  };
}

function toggleBtn(active: boolean, width = 55, height = 26): React.CSSProperties {
  return {
    ...btnGray(width, height),
    background: active ? "#1BFF16" : BTN_GRAY,
  };
}

const S: Record<string, React.CSSProperties> = {
  root: {
    background: BG,
    width: 529,
    maxWidth: "95vw",
    padding: "6px 22px 16px",
    fontFamily: FONT,
    color: "#000",
    display: "flex",
    flexDirection: "column",
    gap: 10,
    overflowY: "auto",
    maxHeight: "95vh",
  },
  title: {
    fontSize: 16,
    fontWeight: 300,
    textAlign: "center",
    marginBottom: 0,
    fontFamily: FONT,
  },
  section: {
    border: "1px solid black",
    padding: "6px 12px 8px",
  },
  label: {
    fontSize: 14,
    fontWeight: 300,
    fontFamily: FONT,
    marginBottom: 4,
    display: "block",
  },
  row: {
    display: "flex",
    alignItems: "center",
    gap: 8,
  },
  inputWide: {
    width: 195,
    height: 25,
    background: INPUT_BG,
    border: "1px solid black",
    fontSize: 14,
    fontFamily: FONT,
    padding: "0 6px",
    flexShrink: 0,
    textTransform: "uppercase" as const,
  },
  inputSmall: {
    width: 68,
    height: 25,
    background: INPUT_BG,
    border: "1px solid black",
    fontSize: 20,
    fontFamily: FONT,
    padding: "0 6px",
    flexShrink: 0,
  },
  inputRemarks: {
    width: 361,
    height: 25,
    background: INPUT_BG,
    border: "1px solid black",
    fontSize: 14,
    fontFamily: FONT,
    padding: "0 6px",
    flexShrink: 0,
  },
  displayBox: {
    width: 68,
    height: 25,
    background: INPUT_BG,
    border: "1px solid black",
    fontSize: 20,
    fontFamily: FONT,
    display: "flex",
    alignItems: "center",
    paddingLeft: 6,
    flexShrink: 0,
  },
  spacer: { flex: 1 },
  presetsRow: {
    display: "flex",
    gap: 4,
    marginTop: 8,
    flexWrap: "wrap" as const,
  },
  numpadContainer: {
    display: "grid",
    gridTemplateColumns: "repeat(3, 26px)",
    gap: 4,
    flexShrink: 0,
  },
  numKey: {
    width: 26,
    height: 26,
    background: BTN_GRAY,
    border: "none",
    fontSize: 14,
    fontWeight: 600,
    fontFamily: FONT,
    cursor: "pointer",
    boxShadow: DROP_SHADOW,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
  },
  footer: {
    display: "flex",
    justifyContent: "space-between",
    marginTop: 4,
  },
  btnDark: {
    width: 66,
    height: 37,
    background: BTN_DARK,
    border: "none",
    fontSize: 24,
    fontWeight: 600,
    fontFamily: FONT,
    color: "#fff",
    cursor: "pointer",
    boxShadow: DROP_SHADOW,
  },
  btnDarkDisabled: {
    width: 66,
    height: 37,
    background: "#888",
    border: "none",
    fontSize: 24,
    fontWeight: 600,
    fontFamily: FONT,
    color: "#bbb",
    cursor: "not-allowed",
    boxShadow: DROP_SHADOW,
  },
};

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialCallsign?: string;
}

export function NewVfrDialog({ open, onOpenChange, initialCallsign = "" }: Props) {
  const strips = useStrips();
  const createVFRFPL = useWebSocketStore(s => s.createVFRFPL);
  const rawMetar = useMetar();

  const [callsign, setCallsign] = useState(initialCallsign);
  const [callsignError, setCallsignError] = useState<string | null>(null);
  const [aircraftType, setAircraftType] = useState("");
  const [personsOnBoard, setPersonsOnBoard] = useState("");
  const [ssr, setSsr] = useState("7000");
  const [fplType, setFplType] = useState<"FPL" | "APL">("FPL");
  const [language, setLanguage] = useState<"DK" | "ENG">("DK");
  const [remarks, setRemarks] = useState("");
  const [qnhDisplay, setQnhDisplay] = useState("XXXX");

  useEffect(() => {
    if (open) {
      const cs = initialCallsign.toUpperCase();
      setCallsign(cs);
      setCallsignError(null);
      setAircraftType("");
      setPersonsOnBoard("");
      setSsr("7000");
      setFplType("FPL");
      setLanguage("DK");
      setRemarks("");
      refreshQnh();
      if (cs) validateCallsign(cs);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, initialCallsign]);

  function refreshQnh() {
    const decoded = decodeMetar(rawMetar);
    if (decoded.qnh != null) {
      setQnhDisplay(String(decoded.qnh));
    } else {
      setQnhDisplay("XXXX");
    }
  }

  function validateCallsign(cs: string) {
    const found = strips.find(s => s.callsign.toUpperCase() === cs.toUpperCase());
    setCallsignError(found ? null : "Callsign not connected");
  }

  function handleCallsignBlur() {
    if (!callsign.trim()) { setCallsignError(null); return; }
    validateCallsign(callsign.trim());
  }

  function appendPOB(digit: number) {
    setPersonsOnBoard(prev => {
      const next = prev + String(digit);
      return next.length > 3 ? prev : next;
    });
  }

  function appendRemark(preset: string) {
    setRemarks(prev => prev ? `${prev} ${preset}` : preset);
  }

  function handleOk() {
    if (callsignError || !callsign.trim()) return;
    createVFRFPL(
      callsign.trim().toUpperCase(),
      aircraftType.trim().toUpperCase(),
      personsOnBoard ? parseInt(personsOnBoard, 10) : 0,
      ssr.trim() || "7000",
      fplType,
      language,
      remarks.trim(),
    );
    onOpenChange(false);
  }

  const canSubmit = !callsignError && callsign.trim().length > 0;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent style={S.root}>
        <DialogTitle style={S.title}>NEW VFR FLIGHT</DialogTitle>

        {/* CALLSIGN */}
        <div style={S.section}>
          <span style={S.label}>CALLSIGN</span>
          <div style={S.row}>
            <input
              style={S.inputWide}
              value={callsign}
              onChange={e => setCallsign(e.target.value.toUpperCase())}
              onBlur={handleCallsignBlur}
              autoFocus
            />
            <div style={S.spacer} />
            <button style={btnGray(110)} onClick={() => { setCallsign(""); setCallsignError(null); }}>ERASE</button>
          </div>
          {callsignError && (
            <div style={{ color: "#cc0000", fontSize: 12, marginTop: 4 }}>{callsignError}</div>
          )}
        </div>

        {/* AIRCRAFT TYPE */}
        <div style={S.section}>
          <span style={S.label}>AIRCRAFT TYPE</span>
          <div style={S.row}>
            <input
              style={S.inputWide}
              value={aircraftType}
              onChange={e => setAircraftType(e.target.value.toUpperCase())}
            />
            <div style={S.spacer} />
            <button style={btnGray(110)} onClick={() => setAircraftType("")}>ERASE</button>
          </div>
          <div style={S.presetsRow}>
            {["PA28", "C172", "DA42", "SR22", "PA34"].map(t => (
              <button key={t} style={btnGray(55)} onClick={() => setAircraftType(t)}>{t}</button>
            ))}
          </div>
        </div>

        {/* PERSONS ONBOARD */}
        <div style={{ ...S.section, display: "flex", gap: 16, alignItems: "flex-start" }}>
          <div style={{ flex: 1 }}>
            <span style={S.label}>PERSONS ONBOARD</span>
            <div style={S.row}>
              <div style={S.displayBox}>{personsOnBoard || "X"}</div>
              <button style={btnGray(64)} onClick={() => setPersonsOnBoard("")}>ERASE</button>
            </div>
          </div>
          <div>
            <div style={S.numpadContainer}>
              {[1, 2, 3, 4, 5, 6, 7, 8, 9].map(n => (
                <button key={n} style={S.numKey} onClick={() => appendPOB(n)}>{n}</button>
              ))}
              <div />
              <button style={S.numKey} onClick={() => appendPOB(0)}>0</button>
              <div />
            </div>
          </div>
        </div>

        {/* QNH */}
        <div style={S.section}>
          <span style={S.label}>QNH</span>
          <div style={S.row}>
            <div style={S.displayBox}>{qnhDisplay}</div>
            <div style={S.spacer} />
            <button style={btnGray(110)} onClick={refreshQnh}>UPDATE</button>
          </div>
        </div>

        {/* TRANSPONDER CODE */}
        <div style={S.section}>
          <span style={S.label}>TRANSPONDER CODE</span>
          <div style={S.row}>
            <input
              style={S.inputSmall}
              value={ssr}
              onChange={e => setSsr(e.target.value)}
              maxLength={4}
            />
            <div style={S.spacer} />
            <button style={btnGray(55)} onClick={() => setSsr("7000")}>ERASE</button>
          </div>
        </div>

        {/* FLIGHTPLAN */}
        <div style={S.section}>
          <span style={S.label}>FLIGHTPLAN</span>
          <div style={S.row}>
            <div style={S.displayBox}>{fplType}</div>
            <div style={S.spacer} />
            <button style={toggleBtn(fplType === "APL")} onClick={() => setFplType("APL")}>APL</button>
            <button style={toggleBtn(fplType === "FPL")} onClick={() => setFplType("FPL")}>FPL</button>
          </div>
        </div>

        {/* LANGUAGE */}
        <div style={S.section}>
          <span style={S.label}>LANGUAGE</span>
          <div style={S.row}>
            <div style={S.displayBox}>{language}</div>
            <div style={S.spacer} />
            <button style={toggleBtn(language === "DK")} onClick={() => setLanguage("DK")}>DK</button>
            <button style={toggleBtn(language === "ENG")} onClick={() => setLanguage("ENG")}>ENG</button>
          </div>
        </div>

        {/* REMARKS */}
        <div style={S.section}>
          <span style={S.label}>REMARKS</span>
          <div style={S.row}>
            <input
              style={S.inputRemarks}
              value={remarks}
              onChange={e => setRemarks(e.target.value)}
            />
            <div style={S.spacer} />
            <button style={btnGray(73)} onClick={() => setRemarks("")}>ERASE</button>
          </div>
          <div style={S.presetsRow}>
            {["CIRCUIT", "RIGET", "SIGHT...", "+ S-VFR"].map(p => (
              <button key={p} style={btnGray(100)} onClick={() => appendRemark(p)}>{p}</button>
            ))}
          </div>
          <div style={{ ...S.presetsRow, marginTop: 4 }}>
            {["VALLENBÆK", "ELLEHAMMER", "NORDHAVN"].map(p => (
              <button key={p} style={btnGray(132)} onClick={() => appendRemark(p)}>{p}</button>
            ))}
          </div>
        </div>

        {/* Footer */}
        <div style={S.footer}>
          <button style={S.btnDark} onClick={() => onOpenChange(false)}>ESC</button>
          <button
            style={canSubmit ? S.btnDark : S.btnDarkDisabled}
            disabled={!canSubmit}
            onClick={handleOk}
          >
            OK
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
