import { useState } from "react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { useStrips } from "@/store/store-hooks";
import { NewIfrDialog } from "./NewIfrDialog";

const DROP_SHADOW = "0 4px 4px rgba(0,0,0,0.25)";

const S: Record<string, React.CSSProperties> = {
  root: {
    background: "#B3B3B3",
    border: "1px solid black",
    color: "#000",
    width: "min(360px, 95vw)",
    padding: "16px 20px",
    display: "flex",
    flexDirection: "column",
    gap: 10,
    fontFamily: "'Arial', sans-serif",
  },
  title: { fontWeight: "bold", fontSize: 18, textAlign: "center", marginBottom: 4 },
  row: { display: "flex", alignItems: "center", gap: 8 },
  input: {
    flex: 1,
    height: 36,
    background: "#FCFCFC",
    border: "1px solid black",
    fontSize: 16,
    padding: "0 8px",
    boxShadow: DROP_SHADOW,
    textTransform: "uppercase" as const,
  },
  notFound: { color: "#cc0000", fontSize: 14, textAlign: "center" as const },
  btn: {
    height: 36,
    minWidth: 80,
    fontSize: 14,
    fontWeight: "bold",
    border: "2px solid white",
    background: "#646464",
    color: "#fff",
    cursor: "pointer",
    boxShadow: DROP_SHADOW,
  },
  btnRow: { display: "flex", gap: 12, justifyContent: "center" },
};

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type State = "idle" | "not_found";

export function PlannedDialog({ open, onOpenChange }: Props) {
  const strips = useStrips();
  const [callsign, setCallsign] = useState("");
  const [state, setState] = useState<State>("idle");
  const [newOpen, setNewOpen] = useState(false);
  const [newCallsign, setNewCallsign] = useState("");

  function handleSearch() {
    const cs = callsign.trim().toUpperCase();
    if (!cs) return;
    const found = strips.find(s => s.callsign.toUpperCase() === cs);
    if (found) {
      // Strip already visible in its bay — just close
      onOpenChange(false);
      setCallsign("");
      setState("idle");
    } else {
      setState("not_found");
    }
  }

  function handleClose() {
    setCallsign("");
    setState("idle");
    onOpenChange(false);
  }

  function handleNew() {
    setNewCallsign(callsign.trim().toUpperCase());
    setCallsign("");
    setState("idle");
    onOpenChange(false);
    setNewOpen(true);
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter") handleSearch();
  }

  return (
    <>
      <Dialog open={open} onOpenChange={(v) => { if (!v) handleClose(); }}>
        <DialogContent style={S.root}>
          <DialogTitle style={S.title}>PLANNED</DialogTitle>

          <div style={S.row}>
            <input
              style={S.input}
              value={callsign}
              onChange={e => { setCallsign(e.target.value.toUpperCase()); setState("idle"); }}
              onKeyDown={handleKeyDown}
              placeholder="Callsign"
              autoFocus
            />
            <button style={S.btn} onClick={handleSearch}>SEARCH</button>
          </div>

          {state === "not_found" && (
            <>
              <div style={S.notFound}>Not found</div>
              <div style={S.btnRow}>
                <button style={S.btn} onClick={handleClose}>CLOSE</button>
                <button style={S.btn} onClick={handleNew}>NEW</button>
              </div>
            </>
          )}

          {state === "idle" && (
            <div style={S.btnRow}>
              <button style={S.btn} onClick={handleClose}>CLOSE</button>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} initialCallsign={newCallsign} />
    </>
  );
}
