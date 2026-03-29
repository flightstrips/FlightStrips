import {
  ClipboardList,
  Compass,
  MapPin,
  MessageSquare,
  Navigation,
  Plane,
  Radio,
  Route,
  Signal,
  TrendingUp,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import {
  hasParsedRows,
  parsePdcClearance,
  type ParsedPdcClearance,
} from "@/lib/pdcClearanceParse";

type ViewMode = "simple" | "raw";

type RowProps = {
  icon: ReactNode;
  label: string;
  value: string;
};

function Row({ icon, label, value }: RowProps) {
  return (
    <div className="flex gap-3 py-2.5">
      <div
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-zinc-800/80 text-teal-400/90"
        aria-hidden
      >
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-[10px] uppercase tracking-wider text-zinc-500 mb-0.5">
          {label}
        </p>
        <p className="text-sm text-zinc-100 font-medium break-words">{value}</p>
      </div>
    </div>
  );
}

function buildRows(p: ParsedPdcClearance): { key: string; icon: ReactNode; label: string; value: string }[] {
  const rows: { key: string; icon: ReactNode; label: string; value: string }[] =
    [];
  const add = (
    key: string,
    icon: ReactNode,
    label: string,
    value: string | undefined
  ) => {
    if (value?.trim()) rows.push({ key, icon, label, value: value.trim() });
  };

  add("cs", <Plane className="h-4 w-4" />, "Callsign", p.callsign);
  add("to", <Navigation className="h-4 w-4" />, "Cleared to", p.clearedTo);
  add("rwy", <MapPin className="h-4 w-4" />, "Runway", p.runway);
  add("hdg", <Compass className="h-4 w-4" />, "Heading", p.heading);
  add("clb", <TrendingUp className="h-4 w-4" />, "Climb", p.climbTo);
  add("vec", <Route className="h-4 w-4" />, "Vectors", p.vectors);
  add("sid", <Signal className="h-4 w-4" />, "SID", p.sid);
  add("sq", <Radio className="h-4 w-4" />, "Squawk", p.squawk);
  add("atis", <ClipboardList className="h-4 w-4" />, "ATIS", p.atis);
  add("next", <Radio className="h-4 w-4" />, "Next frequency", p.nextFrequency);
  add("dep", <Radio className="h-4 w-4" />, "Departure frequency", p.departureFrequency);
  add("rmk", <MessageSquare className="h-4 w-4" />, "Remarks", p.remarks);

  return rows;
}

type Props = {
  /** Full message including optional /data2/ wrapper */
  clearanceText: string;
  /** Fallback when text is missing or placeholder */
  displayFallback: string;
};

export function PdcClearancePanel({ clearanceText, displayFallback }: Props) {
  const [mode, setMode] = useState<ViewMode>("simple");

  const parsed = useMemo(
    () => parsePdcClearance(clearanceText || ""),
    [clearanceText]
  );
  const rows = useMemo(() => buildRows(parsed), [parsed]);
  const showSimple = hasParsedRows(parsed) && rows.length > 0;
  const rawBody = clearanceText?.trim() || displayFallback;

  useEffect(() => {
    if (!showSimple) setMode("raw");
  }, [showSimple]);

  return (
    <div className="rounded-md border border-zinc-700 bg-zinc-950/80 p-4 min-w-0">
      <div className="flex flex-wrap items-center justify-between gap-2 mb-3">
        <p className="text-xs text-zinc-500">Clearance</p>
        <div
          className="flex rounded-md border border-zinc-700 p-0.5 bg-zinc-900/80"
          role="tablist"
          aria-label="Clearance display mode"
        >
          <button
            type="button"
            role="tab"
            aria-selected={mode === "simple"}
            onClick={() => setMode("simple")}
            disabled={!showSimple}
            className={`px-3 py-1 text-xs font-medium rounded ${
              mode === "simple"
                ? "bg-teal-800/80 text-white"
                : "text-zinc-400 hover:text-zinc-200"
            } disabled:opacity-40 disabled:cursor-not-allowed`}
          >
            Simple
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={mode === "raw"}
            onClick={() => setMode("raw")}
            className={`px-3 py-1 text-xs font-medium rounded ${
              mode === "raw"
                ? "bg-teal-800/80 text-white"
                : "text-zinc-400 hover:text-zinc-200"
            }`}
          >
            Raw
          </button>
        </div>
      </div>

      {mode === "raw" ? (
        <pre className="text-xs leading-relaxed whitespace-pre-wrap font-mono text-zinc-200 overflow-x-auto">
          {rawBody}
        </pre>
      ) : showSimple ? (
        <div className="space-y-0">
          {parsed.headerSummary ? (
            <p className="text-[11px] text-zinc-500 mb-3 font-mono break-words">
              {parsed.headerSummary}
            </p>
          ) : null}
          <div className="rounded-md border border-zinc-800/60 divide-y divide-zinc-800/80">
            {rows.map((r) => (
              <Row key={r.key} icon={r.icon} label={r.label} value={r.value} />
            ))}
          </div>
          <p className="text-[11px] text-zinc-500 mt-3">
            Check charts for confirmation. Switch to &quot;Raw&quot; for the
            exact datalink text.
          </p>
        </div>
      ) : (
        <pre className="text-xs leading-relaxed whitespace-pre-wrap font-mono text-zinc-200 overflow-x-auto">
          {rawBody}
        </pre>
      )}

      {clearanceText ? (
        <button
          type="button"
          className="mt-3 text-sm text-teal-400 hover:text-teal-300"
          onClick={() => void navigator.clipboard.writeText(clearanceText)}
        >
          Copy full message
        </button>
      ) : null}
    </div>
  );
}
