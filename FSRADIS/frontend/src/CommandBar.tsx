import { useEffect, useState } from "react";
import { useDroppable } from "@dnd-kit/core";

const VATSIM_DATA_URL = "https://data.vatsim.net/v3/vatsim-data.json";

interface VatsimAtisStation {
  callsign: string;
  atis_code?: string;
  text_atis?: string[];
}

interface VatsimDataResponse {
  atis?: VatsimAtisStation[];
}

function parseAtisLetter(
  stations: VatsimAtisStation[],
  airport: string,
): string | null {
  const station = stations.find(
    (entry) =>
      entry.callsign.toUpperCase().startsWith(`${airport}_`) &&
      entry.callsign.toUpperCase().endsWith("_ATIS"),
  );
  return station?.atis_code?.toUpperCase() ?? null;
}

function parseRunwaysInUse(
  stations: VatsimAtisStation[],
  airport: string,
): { departure?: string; arrival?: string } {
  let departure: string | undefined;
  let arrival: string | undefined;
  let generic: string | undefined;

  stations
    .filter((station) =>
      station.callsign.toUpperCase().startsWith(`${airport}_`),
    )
    .forEach((station) => {
      const atisText = (station.text_atis ?? []).join(" ").toUpperCase();
      const runway = atisText.match(
        /\b(?:RWY|RUNWAY)(?:\s+IN\s+USE)?\s+(\d{2}[LRC]?)/,
      )?.[1];
      if (!runway) {
        return;
      }

      const atisIdentifier = `${station.callsign} ${atisText}`;
      if (/(?:_D_ATIS\b|\bDEP(?:ARTURE)?\b)/.test(atisIdentifier)) {
        departure = runway;
      } else if (/(?:_A_ATIS\b|\bARR(?:IVAL)?\b)/.test(atisIdentifier)) {
        arrival = runway;
      } else {
        generic = runway;
      }
    });

  return {
    departure: departure ?? generic,
    arrival: arrival ?? generic,
  };
}

function parseWindCompact(metar: string | null): string {
  if (!metar) {
    return "— / —";
  }
  if (/\b00000KT\b/.test(metar)) {
    return "000 / 00";
  }
  const vrb = metar.match(/\bVRB(\d{2})(?:G\d{2})?KT\b/);
  if (vrb) {
    return `VRB / ${vrb[1]}`;
  }
  const wind = metar.match(/\b(\d{3})(\d{2})(?:G\d{2})?KT\b/);
  if (wind) {
    return `${wind[1]} / ${wind[2]}`;
  }
  return "— / —";
}

function formatUtcTime(date: Date): string {
  return `${String(date.getUTCHours()).padStart(2, "0")}:${String(date.getUTCMinutes()).padStart(2, "0")}:${String(date.getUTCSeconds()).padStart(2, "0")}z`;
}

interface RunwayMenuRow {
  left: string;
  right: string;
}

interface RunwayMenuConfig {
  rows: RunwayMenuRow[];
  topLineScale: number;
  bottomLineScale: number;
}

function getRunwayMenuConfig(airport: string): RunwayMenuConfig {
  switch (airport) {
    case "EKKA":
      return {
        rows: [
          { left: "09L", right: "27R" },
          { left: "09R", right: "27L" },
        ],
        topLineScale: 0.5,
        bottomLineScale: 1,
      };
    case "EKSP":
      return {
        rows: [
          { left: "10L", right: "28R" },
          { left: "10R", right: "28L" },
        ],
        topLineScale: 1,
        bottomLineScale: 0.5,
      };
    case "EKAH":
      return {
        rows: [
          { left: "10L", right: "28R" },
          { left: "10R", right: "28L" },
        ],
        topLineScale: 0.5,
        bottomLineScale: 1,
      };
    case "EKBI":
      return {
        rows: [{ left: "09", right: "27" }],
        topLineScale: 1,
        bottomLineScale: 1,
      };
    default:
      return {
        rows: [
          { left: "08L", right: "26R" },
          { left: "08R", right: "26L" },
        ],
        topLineScale: 1,
        bottomLineScale: 1,
      };
  }
}

interface CommandBarProps {
  onFind: () => void;
  onNew: () => void;
  deleteDropId: string;
  isDraggingStrip: boolean;
  departureRunway: string;
  onDepartureRunwayChange: (runway: string) => void;
  arrivalRunway: string;
  onArrivalRunwayChange: (runway: string) => void;
  airport: string;
  airportOptions: string[];
  onAirportChange: (airport: string) => void;
}

export default function CommandBar({
  onFind,
  onNew,
  deleteDropId,
  isDraggingStrip,
  departureRunway,
  onDepartureRunwayChange,
  arrivalRunway,
  onArrivalRunwayChange,
  airport,
  airportOptions,
  onAirportChange,
}: CommandBarProps) {
  const { setNodeRef: setDeleteRef, isOver: isDeleteOver } = useDroppable({
    id: deleteDropId,
    disabled: !isDraggingStrip,
  });
  const [metar, setMetar] = useState<string | null>(null);
  const [qnh, setQnh] = useState("1015");
  const [wind, setWind] = useState("250 / 17");
  const [atisLetter, setAtisLetter] = useState("-");
  const [utcTime, setUtcTime] = useState(() => formatUtcTime(new Date()));
  const [runwayMenuOpen, setRunwayMenuOpen] = useState<"DEP" | "ARR" | null>(
    null,
  );
  const [airportMenuOpen, setAirportMenuOpen] = useState(false);

  const runwayMenuConfig = getRunwayMenuConfig(airport);
  const runwayPairs = runwayMenuConfig.rows;

  function chooseRunway(runway: string, type: "DEP" | "ARR") {
    if (type === "DEP") {
      onDepartureRunwayChange(runway);
    } else {
      onArrivalRunwayChange(runway);
    }
    setRunwayMenuOpen(null);
  }

  useEffect(() => {
    const intervalId = window.setInterval(() => {
      setUtcTime(formatUtcTime(new Date()));
    }, 1000);

    return () => {
      window.clearInterval(intervalId);
    };
  }, []);

  useEffect(() => {
    let active = true;

    async function loadMetar() {
      try {
        const response = await fetch(`https://metar.vatsim.net/${airport}`);
        const text = await response.text();
        if (!active) {
          return;
        }

        setMetar(text);
        setWind(parseWindCompact(text));

        const qnhMatch = text.match(/\bQ(\d{4})\b/);
        setQnh(qnhMatch ? qnhMatch[1] : "1013");
      } catch {
        if (!active) {
          return;
        }

        setMetar(null);
        setWind("250 / 17");
        setQnh("1015");
      }
    }

    loadMetar();
    const intervalId = window.setInterval(loadMetar, 5 * 60 * 1000);

    return () => {
      active = false;
      window.clearInterval(intervalId);
    };
  }, [airport]);

  useEffect(() => {
    let active = true;

    async function loadAtis() {
      try {
        const response = await fetch(VATSIM_DATA_URL);
        if (!response.ok) {
          throw new Error(`VATSIM feed request failed: ${response.status}`);
        }

        const payload = (await response.json()) as VatsimDataResponse;
        const letter = parseAtisLetter(payload.atis ?? [], airport);
        const runways = parseRunwaysInUse(payload.atis ?? [], airport);
        if (!active) {
          return;
        }

        setAtisLetter(letter ?? "-");
        if (runways.departure) {
          onDepartureRunwayChange(runways.departure);
        }
        if (runways.arrival) {
          onArrivalRunwayChange(runways.arrival);
        }
      } catch {
        if (!active) {
          return;
        }

        setAtisLetter("-");
      }
    }

    loadAtis();
    const intervalId = window.setInterval(loadAtis, 15_000);

    return () => {
      active = false;
      window.clearInterval(intervalId);
    };
  }, [airport, onArrivalRunwayChange, onDepartureRunwayChange]);

  return (
    <footer className="command-bar" aria-label="Command bar">
      <div className="command-bar-left">
        <button
          type="button"
          className="command-bar-scope"
          onClick={() => setAirportMenuOpen(true)}
          aria-label="Select controlled airport"
        >
          <span className="command-bar-scope-title">{airport}</span>
        </button>

        <span className="command-bar-label">DEP</span>
        <button
          type="button"
          className={`command-bar-value command-bar-value-light command-bar-value-clickable ${departureRunway === "-" ? "is-unset" : ""}`}
          onClick={() => setRunwayMenuOpen("DEP")}
          aria-label="Select departure runway"
        >
          {departureRunway}
        </button>

        <span className="command-bar-label">ARR</span>
        <button
          type="button"
          className={`command-bar-value command-bar-value-light command-bar-value-clickable ${arrivalRunway === "-" ? "is-unset" : ""}`}
          onClick={() => setRunwayMenuOpen("ARR")}
          aria-label="Select arrival runway"
        >
          {arrivalRunway}
        </button>

        <span className="command-bar-label">QNH</span>
        <button
          type="button"
          className="command-bar-value command-bar-value-dark command-bar-value-clickable"
          onClick={() => void 0}
          aria-label="Current QNH"
        >
          {qnh}
        </button>

        <span className="command-bar-value command-bar-value-light command-bar-value-mirror">
          {qnh}
        </span>

        <span className="command-bar-code">{atisLetter}</span>

        <span className="command-bar-value command-bar-value-light command-bar-wind">
          {wind}
        </span>
      </div>

      <div className="command-bar-right">
        <button
          type="button"
          className="command-bar-action command-bar-action-find"
          onClick={onFind}
        >
          PLANNED
        </button>
        <button type="button" className="command-bar-action" onClick={onNew}>
          NEW
        </button>
        <button
          ref={setDeleteRef}
          type="button"
          className={`command-bar-action command-bar-action-x ${isDraggingStrip ? "is-drag-drop-active" : ""} ${isDeleteOver ? "is-drag-drop-over" : ""}`}
          aria-label="Drop strip here to hide"
        >
          X
        </button>
        <div className="command-bar-time">{utcTime}</div>
      </div>

      {runwayMenuOpen && (
        <div
          className="arrival-runway-menu-overlay"
          role="dialog"
          aria-modal="true"
          aria-label={
            runwayMenuOpen === "DEP"
              ? "Departure runway selector"
              : "Arrival runway selector"
          }
        >
          <div className="arrival-runway-menu-frame">
            <div className="arrival-runway-menu-inner">
              {runwayPairs.map((pair, index) => (
                <div
                  key={`${pair.left}-${pair.right}`}
                  className={`arrival-runway-row arrival-runway-row-${index + 1}`}
                >
                  <button
                    type="button"
                    className="arrival-runway-box"
                    onClick={() => chooseRunway(pair.left, runwayMenuOpen)}
                  >
                    {pair.left}
                  </button>
                  <div
                    className="arrival-runway-line"
                    aria-hidden
                    style={{
                      height: `${(index === 0 ? runwayMenuConfig.topLineScale : runwayMenuConfig.bottomLineScale) * 10}px`,
                    }}
                  />
                  <button
                    type="button"
                    className="arrival-runway-box"
                    onClick={() => chooseRunway(pair.right, runwayMenuOpen)}
                  >
                    {pair.right}
                  </button>
                </div>
              ))}

              <button
                type="button"
                className="arrival-runway-esc"
                onClick={() => setRunwayMenuOpen(null)}
              >
                ESC
              </button>
            </div>
          </div>
        </div>
      )}

      {airportMenuOpen && (
        <div
          className="menu-overlay"
          role="dialog"
          aria-modal="true"
          aria-label="Airport selector"
        >
          <div className="menu-panel">
            <div className="menu-title">SELECT AIRPORT</div>
            <div className="menu-grid menu-grid-alt">
              {airportOptions.map((option) => (
                <button
                  key={option}
                  type="button"
                  className={`menu-key ${option === airport ? "is-selected" : ""}`}
                  onClick={() => {
                    onAirportChange(option);
                    setAirportMenuOpen(false);
                  }}
                >
                  {option}
                </button>
              ))}
            </div>
            <div className="menu-actions-rwy">
              <button
                type="button"
                className="menu-action menu-action-esc"
                onClick={() => setAirportMenuOpen(false)}
              >
                ESC
              </button>
            </div>
          </div>
        </div>
      )}
    </footer>
  );
}
