import { useState } from "react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { useStrips } from "@/store/store-hooks";
import { NewIfrDialog } from "./NewIfrDialog";
import FlightPlanDialog from "@/components/FlightPlanDialog";

const DROP_SHADOW = "0 4px 4px rgba(0,0,0,0.25)";

const S: Record<string, React.CSSProperties> = {
  root: {
    background: "#E4E4E4",
    border: "1px solid black",
    color: "#000",
    width: 378,
    padding: "13px 15px 17px",
    display: "flex",
    flexDirection: "column",
    gap: 0,
    fontFamily: "'Rubik', sans-serif",
  },
  title: { fontWeight: 300, fontSize: 24, textAlign: "center", marginBottom: 8 },
  inner: {
    border: "1px solid black",
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    justifyContent: "center",
    gap: 16,
    padding: "20px 16px",
    minHeight: 180,
  },
  label: { fontSize: 20, fontWeight: 300, textAlign: "center" as const, alignSelf: "flex-start" },
  input: {
    width: "100%",
    height: 50,
    background: "#FCFCFC",
    border: "1px solid black",
    fontSize: 24,
    padding: "0 8px",
    boxShadow: DROP_SHADOW,
    textTransform: "uppercase" as const,
    fontFamily: "'Rubik', sans-serif",
  },
  noFpl: { color: "#FF0000", fontSize: 32, textAlign: "center" as const, lineHeight: 1.4, fontWeight: 400 },
  btn: {
    height: 70,
    width: 149,
    fontSize: 32,
    fontWeight: 600,
    border: "none",
    background: "#3F3F3F",
    color: "#fff",
    cursor: "pointer",
    boxShadow: DROP_SHADOW,
  },
  btnRow: { display: "flex", gap: 0, justifyContent: "space-between", width: "100%", paddingTop: 8 },
};

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type State = "idle" | "no_fp" | "open_fp";

export function PlannedDialog({ open, onOpenChange }: Props) {
  const strips = useStrips();
  const [callsign, setCallsign] = useState("");
  const [state, setState] = useState<State>("idle");
  const [foundCallsign, setFoundCallsign] = useState("");
  const [newOpen, setNewOpen] = useState(false);
  const [newCallsign, setNewCallsign] = useState("");

  function handleSearch() {
    const cs = callsign.trim().toUpperCase();
    if (!cs) return;
    const found = strips.find(s => s.callsign.toUpperCase() === cs);
    if (found && found.has_fp !== false) {
      setFoundCallsign(cs);
      setState("open_fp");
    } else {
      setState("no_fp");
    }
  }

  function handleClose() {
    setCallsign("");
    setState("idle");
    setFoundCallsign("");
    onOpenChange(false);
  }

  function handleNew() {
    setNewCallsign(callsign.trim().toUpperCase());
    setCallsign("");
    setState("idle");
    setFoundCallsign("");
    onOpenChange(false);
    setNewOpen(true);
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter") handleSearch();
  }

  if (state === "open_fp") {
    return (
      <>
        <FlightPlanDialog
          callsign={foundCallsign}
          open={true}
          onOpenChange={(v) => {
            if (!v) {
              setState("idle");
              setCallsign("");
              setFoundCallsign("");
              onOpenChange(false);
            }
          }}
        />
        <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} initialCallsign={newCallsign} />
      </>
    );
  }

  return (
    <>
      <Dialog open={open} onOpenChange={(v) => { if (!v) handleClose(); }}>
        <DialogContent style={S.root}>
          <DialogTitle style={S.title}>CREATE/EDIT</DialogTitle>
          <div style={S.inner}>
            {state !== "no_fp" && (
              <>
                <div style={S.label}>C/S</div>
                <input
                  style={S.input}
                  value={callsign}
                  onChange={e => { setCallsign(e.target.value.toUpperCase()); setState("idle"); }}
                  onKeyDown={handleKeyDown}
                  autoFocus
                />
              </>
            )}

            {state === "no_fp" && (
              <div style={S.noFpl}>
                No FPL in system<br />
                Press NEW to make a<br />
                new FPL
              </div>
            )}

            <div style={S.btnRow}>
              <button style={S.btn} onClick={handleClose}>ESC</button>
              {state === "no_fp"
                ? <button style={S.btn} onClick={handleNew}>NEW</button>
                : <button style={S.btn} onClick={handleSearch}>SEARCH</button>
              }
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} initialCallsign={newCallsign} />
    </>
  );
}
