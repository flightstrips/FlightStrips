import { useEffect, useState, type CSSProperties } from "react";

const TARGET_AIRPORT = "EKYT";

type StripRecord = {
  callsign: string;
  departure: string;
  arrival: string;
  aircraft: string;
  wakeTurbulence: string;
  route: string;
  firstWaypoint: string;
  firstAirwayOrSecondPoint: string;
  altitude: string;
  depTime: string;
  hrsEnroute: string;
  minEnroute: string;
  assignedSquawk: string;
  stripType: "ARRIVAL" | "DEPARTURE";
  source: "vatsim" | "fake" | "manual";
};

type FlightPlanSeed = {
  callsign: string;
  departure: string;
  arrival: string;
  aircraft: string;
  wakeTurbulence: string;
  route: string;
  altitude: string;
  depTime: string;
  hrsEnroute: string;
  minEnroute: string;
  assignedSquawk: string;
  source: "vatsim" | "fake" | "manual";
};

interface FindDialogProps {
  open: boolean;
  strips: Record<string, StripRecord>;
  hiddenCallsigns: Set<string>;
  onOpenChange: (open: boolean) => void;
  onOpenExisting: (callsign: string) => void;
  onRequestNew: (callsign: string) => void;
  onHideCallsign: (callsign: string) => void;
}

interface NewFplDialogProps {
  open: boolean;
  initialCallsign: string;
  initialStrip?: StripRecord | null;
  onOpenChange: (open: boolean) => void;
  onCreateStrip: (seed: FlightPlanSeed) => void;
}

function normalizeCallsign(value: string): string {
  return value.trim().toUpperCase();
}

function formatUtcTime(date: Date): string {
  return `${String(date.getUTCHours()).padStart(2, "0")}${String(date.getUTCMinutes()).padStart(2, "0")}`;
}

function fieldStyle(): CSSProperties {
  return {
    width: "100%",
    height: 38,
    border: "1px solid #111",
    background: "#f7f7f7",
    color: "#111",
    padding: "0 10px",
    fontSize: 18,
    fontFamily: "Rubik, Arial, sans-serif",
    textTransform: "uppercase",
    outline: "none",
  };
}

function panelButtonStyle(kind: "primary" | "dark" | "light" = "dark"): CSSProperties {
  const background = kind === "primary" ? "#1bff16" : kind === "light" ? "#dedede" : "#3f3f3f";
  const color = kind === "primary" || kind === "light" ? "#111" : "#fff";

  return {
    minWidth: 96,
    height: 44,
    border: "1px solid #111",
    background,
    color,
    fontSize: 18,
    fontWeight: 700,
    fontFamily: "Rubik, Arial, sans-serif",
    cursor: "pointer",
  };
}

function dialogShellStyle(): CSSProperties {
  return {
    position: "fixed",
    inset: 0,
    zIndex: 120,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    background: "rgba(0, 0, 0, 0.35)",
    padding: 16,
  };
}

function dialogPanelStyle(width: number): CSSProperties {
  return {
    width,
    maxWidth: "calc(100vw - 32px)",
    border: "1px solid #111",
    background: "#d4d4d4",
    color: "#111",
    fontFamily: "Rubik, Arial, sans-serif",
    boxShadow: "0 12px 28px rgba(0, 0, 0, 0.35)",
    padding: 18,
  };
}

export function FindDialog({
  open,
  strips,
  hiddenCallsigns,
  onOpenChange,
  onOpenExisting,
  onRequestNew,
  onHideCallsign,
}: FindDialogProps) {
  const [query, setQuery] = useState("");
  const [foundCallsign, setFoundCallsign] = useState<string | null>(null);
  const [status, setStatus] = useState<"idle" | "not-found">("idle");

  useEffect(() => {
    if (!open) {
      setQuery("");
      setFoundCallsign(null);
      setStatus("idle");
    }
  }, [open]);

  function searchCallsign() {
    const callsign = normalizeCallsign(query);
    if (!callsign) {
      return;
    }

    const match = Object.values(strips).find((strip) => strip.callsign.toUpperCase() === callsign);
    if (!match) {
      setFoundCallsign(null);
      setStatus("not-found");
      return;
    }

    setFoundCallsign(match.callsign.toUpperCase());
    setStatus("idle");
  }

  function handleOpenExisting() {
    if (!foundCallsign) {
      return;
    }

    onOpenExisting(foundCallsign);
    onOpenChange(false);
  }

  function handleHide() {
    if (!foundCallsign) {
      return;
    }

    onHideCallsign(foundCallsign);
    onOpenChange(false);
  }

  function handleNew() {
    onRequestNew(normalizeCallsign(query));
    onOpenChange(false);
  }

  if (!open) {
    return null;
  }

  const isFoundHidden = foundCallsign ? hiddenCallsigns.has(foundCallsign) : false;

  return (
    <div style={dialogShellStyle()} role="dialog" aria-modal="true" aria-label="Find callsign dialog">
      <div style={dialogPanelStyle(404)}>
        <div style={{ fontSize: 28, fontWeight: 300, textAlign: "center", marginBottom: 12 }}>CREATE / EDIT</div>

        {foundCallsign ? (
          <div style={{ border: "1px solid #111", background: "#ededed", padding: 16, minHeight: 160, display: "flex", flexDirection: "column", gap: 12, justifyContent: "center" }}>
            <div style={{ fontSize: 18, fontWeight: 300, textAlign: "center" }}>C/S</div>
            <div style={{ ...fieldStyle(), display: "flex", alignItems: "center", justifyContent: "center", background: "#fff" }}>{foundCallsign}</div>
            <div style={{ fontSize: 18, textAlign: "center", fontWeight: 700 }}>
              {isFoundHidden ? "STORED IN HIDDEN STRIPS" : "FPL FOUND"}
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", gap: 8 }}>
              <button type="button" style={panelButtonStyle("dark")} onClick={() => onOpenChange(false)}>ESC</button>
              <button type="button" style={panelButtonStyle("light")} onClick={handleHide}>X</button>
              <button type="button" style={panelButtonStyle("primary")} onClick={handleOpenExisting}>OPEN</button>
              <button type="button" style={panelButtonStyle("dark")} onClick={handleNew}>NEW</button>
            </div>
          </div>
        ) : (
          <div style={{ border: "1px solid #111", background: "#ededed", padding: 16, minHeight: 160, display: "flex", flexDirection: "column", gap: 12, justifyContent: "center" }}>
            <div style={{ fontSize: 18, fontWeight: 300, textAlign: "center" }}>C/S</div>
            <input
              style={fieldStyle()}
              value={query}
              onChange={(event) => {
                setQuery(event.target.value.toUpperCase());
                setStatus("idle");
              }}
              onKeyDown={(event) => {
                if (event.key === "Enter") {
                  searchCallsign();
                }
              }}
              autoFocus
            />
            {status === "not-found" && (
              <div style={{ color: "#c00000", fontSize: 22, textAlign: "center", fontWeight: 400 }}>
                No FPL in system
              </div>
            )}
            <div style={{ display: "flex", justifyContent: "space-between", gap: 8 }}>
              <button type="button" style={panelButtonStyle("dark")} onClick={() => onOpenChange(false)}>ESC</button>
              <button type="button" style={panelButtonStyle("primary")} onClick={searchCallsign}>SEARCH</button>
              <button type="button" style={panelButtonStyle("dark")} onClick={handleNew}>NEW</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

export function NewFplDialog({
  open,
  initialCallsign,
  initialStrip,
  onOpenChange,
  onCreateStrip,
}: NewFplDialogProps) {
  const [callsign, setCallsign] = useState("");
  const [departure, setDeparture] = useState(TARGET_AIRPORT);
  const [arrival, setArrival] = useState("EKCH");
  const [aircraft, setAircraft] = useState("A320");
  const [route, setRoute] = useState("");
  const [altitude, setAltitude] = useState("FL230");
  const [depTime, setDepTime] = useState(formatUtcTime(new Date()));
  const [hrsEnroute, setHrsEnroute] = useState("00");
  const [minEnroute, setMinEnroute] = useState("30");
  const [assignedSquawk, setAssignedSquawk] = useState("");

  useEffect(() => {
    if (!open) {
      return;
    }

    const nextCallsign = normalizeCallsign(initialCallsign || initialStrip?.callsign || "");
    setCallsign(nextCallsign);
    setDeparture(initialStrip?.departure ?? TARGET_AIRPORT);
    setArrival(initialStrip?.arrival ?? "EKCH");
    setAircraft(initialStrip?.aircraft ?? "A320");
    setRoute(initialStrip?.route ?? "");
    setAltitude(initialStrip?.altitude ?? "FL230");
    setDepTime(initialStrip?.depTime ?? formatUtcTime(new Date()));
    setHrsEnroute(initialStrip?.hrsEnroute ?? "00");
    setMinEnroute(initialStrip?.minEnroute ?? "30");
    setAssignedSquawk(initialStrip?.assignedSquawk ?? "");
  }, [initialCallsign, initialStrip, open]);

  if (!open) {
    return null;
  }

  const isEditMode = !!initialStrip;

  function handleSave() {
    const normalizedCallsign = normalizeCallsign(callsign);
    if (!normalizedCallsign) {
      return;
    }

    onCreateStrip({
      callsign: normalizedCallsign,
      departure: departure.trim().toUpperCase() || TARGET_AIRPORT,
      arrival: arrival.trim().toUpperCase() || "EKCH",
      aircraft: aircraft.trim().toUpperCase() || "A320",
      wakeTurbulence: initialStrip?.wakeTurbulence ?? "M",
      route: route.trim().toUpperCase(),
      altitude: altitude.trim().toUpperCase() || "FL230",
      depTime: depTime.replace(/\D/g, "").slice(-4).padStart(4, "0"),
      hrsEnroute: hrsEnroute.replace(/\D/g, "").slice(-2).padStart(2, "0"),
      minEnroute: minEnroute.replace(/\D/g, "").slice(-2).padStart(2, "0"),
      assignedSquawk: assignedSquawk.replace(/\D/g, "").slice(-4),
      source: initialStrip?.source ?? "manual",
    });

    onOpenChange(false);
  }

  return (
    <div style={dialogShellStyle()} role="dialog" aria-modal="true" aria-label="Flight plan dialog">
      <div style={dialogPanelStyle(700)}>
        <div style={{ fontSize: 28, fontWeight: 300, textAlign: "center", marginBottom: 16 }}>
          {isEditMode ? "FLIGHT PLAN" : "NEW FPL"}
        </div>

        <div style={{ display: "grid", gap: 12 }}>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>C/S</span>
            <input style={fieldStyle()} value={callsign} onChange={(event) => setCallsign(event.target.value.toUpperCase())} disabled={isEditMode} />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>DEP</span>
            <input style={fieldStyle()} value={departure} onChange={(event) => setDeparture(event.target.value.toUpperCase())} disabled={isEditMode} />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>ARR</span>
            <input style={fieldStyle()} value={arrival} onChange={(event) => setArrival(event.target.value.toUpperCase())} disabled={isEditMode} />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>A/C</span>
            <input style={fieldStyle()} value={aircraft} onChange={(event) => setAircraft(event.target.value.toUpperCase())} disabled={isEditMode} />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>ROUTE</span>
            <input style={fieldStyle()} value={route} onChange={(event) => setRoute(event.target.value.toUpperCase())} disabled={isEditMode} />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>ALT</span>
            <input style={fieldStyle()} value={altitude} onChange={(event) => setAltitude(event.target.value.toUpperCase())} disabled={isEditMode} />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>EOBT</span>
            <input style={fieldStyle()} value={depTime} onChange={(event) => setDepTime(event.target.value)} disabled={isEditMode} />
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>ENROUTE</span>
            <div style={{ display: "flex", gap: 8 }}>
              <input style={{ ...fieldStyle(), width: 96 }} value={hrsEnroute} onChange={(event) => setHrsEnroute(event.target.value)} disabled={isEditMode} />
              <input style={{ ...fieldStyle(), width: 96 }} value={minEnroute} onChange={(event) => setMinEnroute(event.target.value)} disabled={isEditMode} />
            </div>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "160px 1fr", gap: 12, alignItems: "center" }}>
            <span style={{ fontSize: 18, fontWeight: 300 }}>SSR</span>
            <input style={fieldStyle()} value={assignedSquawk} onChange={(event) => setAssignedSquawk(event.target.value)} disabled={isEditMode} />
          </div>
        </div>

        <div style={{ display: "flex", justifyContent: "space-between", marginTop: 18, gap: 8 }}>
          <button type="button" style={panelButtonStyle("dark")} onClick={() => onOpenChange(false)}>ESC</button>
          {!isEditMode && <button type="button" style={panelButtonStyle("primary")} onClick={handleSave}>SAVE</button>}
        </div>
      </div>
    </div>
  );
}