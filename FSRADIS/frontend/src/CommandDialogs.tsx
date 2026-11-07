import { useEffect, useState, type CSSProperties } from "react";

const VATSIM_DATA_URL = "https://data.vatsim.net/v3/vatsim-data.json";

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
  registration?: string;
  assignedRunway?: string;
  sid?: string;
  rnav?: string;
  radisRemarks?: string;
  vfr?: boolean;
  stripType: "ARRIVAL" | "DEPARTURE";
  source: "vatsim" | "fake" | "manual" | "planned";
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
  registration?: string;
  assignedRunway?: string;
  sid?: string;
  rnav?: string;
  radisRemarks?: string;
  vfr?: boolean;
  source: "vatsim" | "fake" | "manual" | "planned";
};

interface FindDialogProps {
  open: boolean;
  strips: Record<string, StripRecord>;
  onOpenChange: (open: boolean) => void;
  onSelectStrip: (stripId: string) => void;
}

interface NewFplDialogProps {
  open: boolean;
  initialCallsign: string;
  initialStrip?: StripRecord | null;
  departureAirport: string;
  onOpenChange: (open: boolean) => void;
  onCreateStrip: (seed: FlightPlanSeed) => void;
}

interface VatsimPilot {
  callsign?: string;
  flight_plan?: {
    departure?: string;
    arrival?: string;
    aircraft_short?: string;
    route?: string;
    altitude?: string;
    deptime?: string;
    assigned_transponder?: string;
    remarks?: string;
    wake_turbulence?: string;
  };
}

interface VatsimDataResponse {
  pilots?: VatsimPilot[];
}

function parseRegistration(remarks: string | undefined): string {
  return remarks?.match(/\bREG\/([^\s]+)/i)?.[1]?.toUpperCase() ?? "";
}

const EKBI_SIDS: Record<string, string[]> = {
  "09": [
    "ABINO7A",
    "INTET3A",
    "RERPA3A",
    "BAMPI6A",
    "MIKRO6A",
    "ALS7A",
    "ESIRU1A",
  ],
  "27": [
    "ABINO7B",
    "INTET3B",
    "RERPA3B",
    "BAMPI6B",
    "MIKRO6B",
    "ALS7B",
    "ESIRU1B",
  ],
};

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

function panelButtonStyle(
  kind: "primary" | "dark" | "light" = "dark",
): CSSProperties {
  const background =
    kind === "primary" ? "#1bff16" : kind === "light" ? "#dedede" : "#3f3f3f";
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
  onOpenChange,
  onSelectStrip,
}: FindDialogProps) {
  if (!open) {
    return null;
  }

  const plannedStrips = Object.entries(strips);

  return (
    <div
      style={dialogShellStyle()}
      role="dialog"
      aria-modal="true"
      aria-label="Planned flights"
    >
      <div
        style={{
          ...dialogPanelStyle(560),
          maxHeight: "calc(100vh - 32px)",
          display: "grid",
          gridTemplateRows: "auto minmax(0, 1fr)",
          gap: 8,
        }}
      >
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "124px 136px 1fr",
            gap: 8,
          }}
        >
          <button type="button" style={panelButtonStyle("light")}>
            {plannedStrips.length}
          </button>
          <button type="button" style={panelButtonStyle("light")}>
            SORT
          </button>
          <button
            type="button"
            style={panelButtonStyle("light")}
            onClick={() => onOpenChange(false)}
          >
            DISMISS
          </button>
        </div>
        <div
          style={{
            minHeight: 180,
            overflowY: "auto",
            border: "1px solid #555",
            padding: 8,
            display: "grid",
            alignContent: "start",
            gap: 4,
          }}
        >
          {plannedStrips.map(([stripId, strip]) => (
            <button
              key={stripId}
              type="button"
              onClick={() => onSelectStrip(stripId)}
              style={{
                display: "grid",
                gridTemplateColumns: "1fr 92px",
                minHeight: 54,
                border: "1px solid #333",
                background: "#fff",
                color: "#111",
                padding: 0,
                fontFamily: "Rubik, Arial, sans-serif",
                fontSize: 26,
                fontWeight: 700,
                cursor: "pointer",
              }}
            >
              <span
                style={{
                  display: "flex",
                  alignItems: "center",
                  padding: "0 12px",
                  textAlign: "left",
                }}
              >
                {strip.callsign}
              </span>
              <span
                style={{
                  display: "grid",
                  placeItems: "center",
                  borderLeft: "1px solid #333",
                }}
              >
                <svg
                  viewBox="0 0 24 28"
                  width="26"
                  height="30"
                  aria-hidden="true"
                >
                  <path
                    d={
                      strip.stripType === "DEPARTURE"
                        ? "M12 2v18m0 0-7-7m7 7 7-7"
                        : "M12 26V8m0 0-7 7m7-7 7 7"
                    }
                    stroke="currentColor"
                    strokeWidth="3"
                    fill="none"
                  />
                </svg>
              </span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}

export function NewFplDialog({
  open,
  initialCallsign,
  initialStrip,
  departureAirport,
  onOpenChange,
  onCreateStrip,
}: NewFplDialogProps) {
  const [callsign, setCallsign] = useState("");
  const [arrival, setArrival] = useState("");
  const [aircraft, setAircraft] = useState("");
  const [route, setRoute] = useState("");
  const [altitude, setAltitude] = useState("");
  const [depTime, setDepTime] = useState("");
  const [assignedSquawk, setAssignedSquawk] = useState("");
  const [registration, setRegistration] = useState("");
  const [assignedRunway, setAssignedRunway] = useState("");
  const [sid, setSid] = useState("");
  const [radisRemarks, setRadisRemarks] = useState("");
  const [rnav, setRnav] = useState("1");
  const [sidMenuOpen, setSidMenuOpen] = useState(false);
  const [runwayMenuOpen, setRunwayMenuOpen] = useState(false);
  const [vfr, setVfr] = useState(false);

  useEffect(() => {
    if (!open) {
      return;
    }

    const nextCallsign = normalizeCallsign(
      initialCallsign || initialStrip?.callsign || "",
    );
    setCallsign(nextCallsign);
    setArrival(initialStrip?.arrival ?? "");
    setAircraft(initialStrip?.aircraft ?? "");
    setRoute(initialStrip?.route ?? "");
    setAltitude(initialStrip?.altitude ?? "");
    setDepTime(initialStrip?.depTime ?? "");
    setAssignedSquawk(initialStrip?.assignedSquawk ?? "");
    setRegistration(initialStrip?.registration ?? "");
    setAssignedRunway(initialStrip?.assignedRunway ?? "");
    setSid(initialStrip?.sid ?? "");
    setRadisRemarks(initialStrip?.radisRemarks ?? "");
    setRnav("1");
    setVfr(false);
  }, [departureAirport, initialCallsign, initialStrip, open]);

  useEffect(() => {
    const normalizedCallsign = normalizeCallsign(callsign);
    if (!open || initialStrip || normalizedCallsign.length < 3) {
      return;
    }

    let active = true;
    async function loadMatchingFlightPlan() {
      try {
        const response = await fetch(VATSIM_DATA_URL);
        if (!response.ok) {
          return;
        }
        const payload = (await response.json()) as VatsimDataResponse;
        const pilot = payload.pilots?.find(
          (entry) =>
            normalizeCallsign(entry.callsign ?? "") === normalizedCallsign,
        );
        const flightPlan = pilot?.flight_plan;
        if (!active || !flightPlan) {
          return;
        }

        setArrival(flightPlan.arrival?.toUpperCase() || "");
        setAircraft(flightPlan.aircraft_short?.toUpperCase() || "");
        setRoute(flightPlan.route?.toUpperCase() || "");
        setAltitude(flightPlan.altitude?.toUpperCase() || "");
        setDepTime(flightPlan.deptime?.replace(/\D/g, "").slice(-4) || "");
        setAssignedSquawk(
          flightPlan.assigned_transponder?.replace(/\D/g, "").slice(-4) || "",
        );
        setRegistration(parseRegistration(flightPlan.remarks));
      } catch {
        // A manual flight plan remains available when VATSIM cannot be reached.
      }
    }

    void loadMatchingFlightPlan();
    return () => {
      active = false;
    };
  }, [callsign, departureAirport, initialStrip, open]);

  if (!open) {
    return null;
  }

  const isEditMode = !!initialStrip;
  const isEkbiDeparture = departureAirport === "EKBI";
  const sidOptions = EKBI_SIDS[assignedRunway || "27"] ?? [];

  function handleSave() {
    const normalizedCallsign = normalizeCallsign(callsign);
    if (!normalizedCallsign) {
      return;
    }

    onCreateStrip({
      callsign: normalizedCallsign,
      departure: departureAirport,
      arrival: arrival.trim().toUpperCase(),
      aircraft: aircraft.trim().toUpperCase(),
      wakeTurbulence: initialStrip?.wakeTurbulence ?? "M",
      route: route.trim().toUpperCase(),
      altitude: altitude.trim().toUpperCase(),
      depTime: depTime.replace(/\D/g, "").slice(-4),
      hrsEnroute: initialStrip?.hrsEnroute ?? "",
      minEnroute: initialStrip?.minEnroute ?? "",
      assignedSquawk: assignedSquawk.replace(/\D/g, "").slice(-4),
      registration: registration.trim().toUpperCase(),
      assignedRunway,
      sid,
      rnav,
      radisRemarks,
      vfr,
      source: initialStrip?.source ?? "manual",
    });

    onOpenChange(false);
  }

  return (
    <div
      className="clearance-overlay"
      role="dialog"
      aria-modal="true"
      aria-label="New flight plan"
    >
      <section className="clearance-dialog new-fpl-dialog">
        <fieldset className="clearance-section">
          <legend>DEPARTURE</legend>
          <div className="clearance-grid clearance-grid-top">
            <NewFplField
              label="C/S"
              value={callsign}
              onChange={setCallsign}
              disabled={isEditMode}
            />
            <NewFplField
              label="ADES"
              value={arrival}
              onChange={setArrival}
              disabled={isEditMode}
            />
            <NewFplStaticField label="RNAV" value={rnav} />
            {isEkbiDeparture ? (
              <NewFplMenuField
                label="SID"
                value={sid}
                onClick={() => setSidMenuOpen(true)}
              />
            ) : (
              <NewFplStaticField label="SID" value="NIL" />
            )}
            <NewFplField
              label="SSR"
              value={assignedSquawk}
              onChange={setAssignedSquawk}
              disabled={isEditMode}
            />
            <NewFplStaticField label="CTOT" value="0000" />
          </div>
          <div className="clearance-grid clearance-grid-mid">
            <NewFplField
              label="EOBT"
              value={depTime}
              onChange={setDepTime}
              disabled={isEditMode}
            />
            <NewFplMenuField
              label="RWY"
              value={assignedRunway}
              onClick={() => setRunwayMenuOpen(true)}
            />
            <NewFplStaticField label="REA" />
          </div>
          <div className="clearance-grid clearance-grid-flight">
            <NewFplField
              label="TYPE"
              value={aircraft}
              onChange={setAircraft}
              disabled={isEditMode}
            />
            <NewFplField
              label="FL"
              value={altitude}
              onChange={setAltitude}
              disabled={isEditMode}
            />
            <NewFplStaticField label="SPEED" />
            <NewFplStaticField label="STS" />
          </div>
          <NewFplField
            label="ROUTE"
            value={route}
            onChange={setRoute}
            disabled={isEditMode}
            wide
          />
          <NewFplStaticField label="COOPANS REMARKS" wide />
          <NewFplField
            label="RADIS REMARKS"
            value={radisRemarks}
            onChange={setRadisRemarks}
            disabled={isEditMode}
            wide
          />
          <div className="clearance-grid clearance-grid-controls">
            <NewFplStaticField label="CLIMB GR." />
            <NewFplStaticField label="HDG" />
            <NewFplStaticField label="ALT" />
            <NewFplStaticField label="DE-ICE" />
            <NewFplField
              label="REG"
              value={registration}
              onChange={setRegistration}
              disabled={isEditMode}
            />
            <NewFplStaticField label="STAND" />
          </div>
        </fieldset>
        <fieldset className="clearance-section clearance-arrival-section">
          <legend>ARRIVAL</legend>
          <div className="clearance-grid clearance-grid-arrival">
            <NewFplStaticField label="ADEP" />
            <NewFplStaticField label="STAR" />
            <NewFplStaticField label="RWY" />
            <NewFplStaticField label="ETA" />
            <NewFplStaticField label="AOBT" />
          </div>
        </fieldset>
        <div className="new-fpl-actions">
          <button type="button" onClick={() => onOpenChange(false)}>
            ESC
          </button>
          {!isEditMode && (
            <button type="button" onClick={handleSave}>
              OK
            </button>
          )}
        </div>
        {runwayMenuOpen && (
          <div
            className="clearance-sid-menu"
            role="dialog"
            aria-modal="true"
            aria-label="Runway selector"
          >
            <div className="clearance-sid-menu-panel clearance-runway-menu-panel">
              {(isEkbiDeparture ? ["09", "27"] : []).map((runway) => (
                <button
                  key={runway}
                  className={runway === assignedRunway ? "is-selected" : ""}
                  onClick={() => setAssignedRunway(runway)}
                >
                  {runway}
                </button>
              ))}
              <div className="clearance-sid-footer">
                <button onClick={() => setRunwayMenuOpen(false)}>ESC</button>
              </div>
            </div>
          </div>
        )}
        {sidMenuOpen && (
          <div
            className="clearance-sid-menu"
            role="dialog"
            aria-modal="true"
            aria-label="SID selector"
          >
            <div className="clearance-sid-menu-panel">
              <div className="clearance-sid-options">
                {sidOptions.map((option) => (
                  <button
                    key={option}
                    className={option === sid ? "is-selected" : ""}
                    onClick={() => setSid(option)}
                  >
                    {option}
                  </button>
                ))}
              </div>
              <div className="clearance-sid-actions">
                <button
                  type="button"
                  onClick={() => {
                    setSid("VFR");
                    setAssignedSquawk("7000");
                    setVfr(true);
                  }}
                >
                  VFR
                </button>
                <button type="button" onClick={() => setRnav("NON")}>
                  NON RNAV
                </button>
                <button type="button" onClick={() => setRnav("1")}>
                  REMOVE NON RNAV
                </button>
              </div>
              <div className="clearance-sid-footer">
                <button onClick={() => setSid("")}>ERASE</button>
                <button onClick={() => setSidMenuOpen(false)}>ESC</button>
              </div>
            </div>
          </div>
        )}
      </section>
    </div>
  );
}

function NewFplField({
  label,
  value,
  onChange,
  disabled,
  wide = false,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  wide?: boolean;
}) {
  return (
    <label
      className={`clearance-field clearance-field-editable ${wide ? "clearance-field-wide" : ""}`}
    >
      <span>{label}</span>
      <input
        value={value}
        onChange={(event) => onChange(event.target.value.toUpperCase())}
        disabled={disabled}
      />
    </label>
  );
}

function NewFplStaticField({
  label,
  value = "",
  wide = false,
}: {
  label: string;
  value?: string;
  wide?: boolean;
}) {
  return (
    <div
      className={`clearance-field clearance-field-readonly ${wide ? "clearance-field-wide" : ""}`}
    >
      <span>{label}</span>
      <div className="new-fpl-static-value">{value}</div>
    </div>
  );
}

function NewFplMenuField({
  label,
  value,
  onClick,
}: {
  label: string;
  value: string;
  onClick: () => void;
}) {
  return (
    <label className="clearance-field clearance-field-editable">
      <span>{label}</span>
      <button type="button" onClick={onClick}>
        {value}
      </button>
    </label>
  );
}
