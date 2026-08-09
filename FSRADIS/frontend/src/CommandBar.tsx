import { useEffect, useState } from "react";
import { useDroppable } from "@dnd-kit/core";

const TARGET_AIRPORT = "EKYT";

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

interface CommandBarProps {
  onFind: () => void;
  onNew: () => void;
  deleteDropId: string;
  isDraggingStrip: boolean;
  departureRunway: string;
  onDepartureRunwayChange: (runway: string) => void;
  arrivalRunway: string;
  onArrivalRunwayChange: (runway: string) => void;
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
}: CommandBarProps) {
  const { setNodeRef: setDeleteRef, isOver: isDeleteOver } = useDroppable({
    id: deleteDropId,
    disabled: !isDraggingStrip,
  });
  const [metar, setMetar] = useState<string | null>(null);
  const [qnh, setQnh] = useState("1015");
  const [wind, setWind] = useState("250 / 17");
  const [utcTime, setUtcTime] = useState(() => formatUtcTime(new Date()));
  const [runwayMenuOpen, setRunwayMenuOpen] = useState<"DEP" | "ARR" | null>(null);

  const runwayPairs = [
    { left: "08L", right: "26R" },
    { left: "08R", right: "26L" },
  ];

  function chooseRunway(runway: string) {
    if (runwayMenuOpen === "DEP") {
      onDepartureRunwayChange(runway);
    } else if (runwayMenuOpen === "ARR") {
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
        const response = await fetch(`https://metar.vatsim.net/${TARGET_AIRPORT}`);
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
  }, []);

  return (
    <footer className="command-bar" aria-label="Command bar">
      <div className="command-bar-left">
        <button type="button" className="command-bar-scope">
          <span className="command-bar-scope-title">{TARGET_AIRPORT}</span>
        </button>

        <span className="command-bar-label">DEP</span>
        <button
          type="button"
          className="command-bar-value command-bar-value-light command-bar-value-clickable"
          onClick={() => setRunwayMenuOpen("DEP")}
          aria-label="Select departure runway"
        >
          {departureRunway}
        </button>

        <span className="command-bar-label">ARR</span>
        <button
          type="button"
          className="command-bar-value command-bar-value-light command-bar-value-clickable"
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

        <span className="command-bar-value command-bar-value-light command-bar-value-mirror">{qnh}</span>

        <span className="command-bar-code">D</span>

        <span className="command-bar-value command-bar-value-light command-bar-wind">{wind}</span>
      </div>

      <div className="command-bar-right">
        <button type="button" className="command-bar-action command-bar-action-find" onClick={onFind}>FIND</button>
        <button type="button" className="command-bar-action" onClick={onNew}>NEW</button>
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
          aria-label={runwayMenuOpen === "DEP" ? "Departure runway selector" : "Arrival runway selector"}
        >
          <div className="arrival-runway-menu-frame">
            <div className="arrival-runway-menu-inner">
              {runwayPairs.map((pair, index) => (
                <div key={`${pair.left}-${pair.right}`} className={`arrival-runway-row arrival-runway-row-${index + 1}`}>
                  <button
                    type="button"
                    className="arrival-runway-box"
                    onClick={() => chooseRunway(pair.left)}
                  >
                    {pair.left}
                  </button>
                  <div className="arrival-runway-line" aria-hidden />
                  <button
                    type="button"
                    className="arrival-runway-box"
                    onClick={() => chooseRunway(pair.right)}
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
    </footer>
  );
}