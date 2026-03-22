import {
  Cloud,
  CloudFog,
  CloudRain,
  CloudSun,
  Sun,
  CloudLightning,
  Wind,
  Eye,
  Layers,
  Thermometer,
  Gauge,
  HelpCircle,
  type LucideIcon,
} from "lucide-react";

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { useAirport, useMetar } from "@/store/store-hooks";
import { CLS_CMDBTN } from "@/components/strip/shared";
import {
  decodeMetar,
  type MetarDecoded,
  type FlightCategory,
} from "@/lib/metarDecode";

const CLS_DIALOG = "bg-[#e4e4e4] w-[42rem] border-4 border-primary";

const CATEGORY_COLOR: Record<FlightCategory, string> = {
  VFR: "text-green-700",
  MVFR: "text-blue-700",
  IFR: "text-amber-700",
  LIFR: "text-red-700",
  UNKN: "text-gray-600",
};

const FLIGHT_CATEGORY_ICON: Record<FlightCategory, LucideIcon> = {
  VFR: Sun,
  MVFR: CloudSun,
  IFR: Cloud,
  LIFR: CloudFog,
  UNKN: HelpCircle,
};

const ICON_CLASS = "w-8 h-8 text-gray-600 shrink-0";
const LABEL_CLASS = "text-gray-600 font-medium text-base";
const VALUE_CLASS = "text-lg font-semibold text-black";

function FlightCategoryIcon({ category }: { category: FlightCategory }) {
  const Icon = FLIGHT_CATEGORY_ICON[category];
  return <Icon className={`${ICON_CLASS} ${CATEGORY_COLOR[category]}`} />;
}

function CloudConditionIcon({ condition }: { condition: MetarDecoded["condition"] }) {
  switch (condition) {
    case "thunderstorm":
      return <CloudLightning className={ICON_CLASS} />;
    case "fg":
      return <CloudFog className={ICON_CLASS} />;
    case "precip":
      return <CloudRain className={ICON_CLASS} />;
    case "ovc":
      return <Cloud className={ICON_CLASS} />;
    case "bkn":
    case "sct":
      return <CloudSun className={ICON_CLASS} />;
    case "few":
    case "clear":
    default:
      return <Sun className={ICON_CLASS} />;
  }
}

function DecodedGrid({ decoded }: { decoded: MetarDecoded }) {
  const {
    flightCategory,
    flightCategoryLabel,
    temperature,
    dewPoint,
    windSpeedKts,
    windDegrees,
    windDirection,
    visibilityDisplay,
    ceilingFt,
    qnh,
    qnhUnit,
    condition,
  } = decoded;

  const windText =
    windSpeedKts != null && windDegrees != null
      ? `${windDegrees}° / ${windSpeedKts} kt`
      : windDirection === "VRB" && windSpeedKts != null
        ? `VRB / ${windSpeedKts} kt`
        : "—";

  return (
    <div className="grid grid-cols-2 gap-x-8 gap-y-5 text-center">
      <div className="flex flex-col items-center gap-1">
        <div className={LABEL_CLASS}>Flight category</div>
        <div className="flex flex-col items-center gap-1">
          <FlightCategoryIcon category={flightCategory} />
          <div className={`${VALUE_CLASS} ${CATEGORY_COLOR[flightCategory]}`}>{flightCategoryLabel}</div>
        </div>
      </div>
      <div className="flex flex-col items-center gap-1">
        <div className={LABEL_CLASS}>Cloud condition</div>
        <div className="flex flex-col items-center gap-1">
          <CloudConditionIcon condition={condition} />
          <span className={VALUE_CLASS}>
            {condition === "clear"
              ? "Clear"
              : condition === "few"
                ? "Few"
                : condition === "sct"
                  ? "Scattered"
                  : condition === "bkn"
                    ? "Broken"
                    : condition === "ovc"
                      ? "Overcast"
                      : condition === "fg"
                        ? "Fog"
                        : condition === "precip"
                          ? "Precipitation"
                          : condition === "thunderstorm"
                            ? "Thunderstorm"
                            : condition}
          </span>
        </div>
      </div>
      <div className="flex flex-col items-center gap-1">
        <div className={LABEL_CLASS}>Wind</div>
        <div className="flex flex-col items-center gap-1">
          <Wind className={ICON_CLASS} />
          <div className={VALUE_CLASS}>{windText}</div>
        </div>
      </div>
      <div className="flex flex-col items-center gap-1">
        <div className={LABEL_CLASS}>Visibility</div>
        <div className="flex flex-col items-center gap-1">
          <Eye className={ICON_CLASS} />
          <div className={VALUE_CLASS}>{visibilityDisplay}</div>
        </div>
      </div>
      <div className="flex flex-col items-center gap-1">
        <div className={LABEL_CLASS}>Ceiling</div>
        <div className="flex flex-col items-center gap-1">
          <Layers className={ICON_CLASS} />
          <div className={VALUE_CLASS}>{ceilingFt != null ? `${ceilingFt.toLocaleString()} ft` : "—"}</div>
        </div>
      </div>
      <div className="flex flex-col items-center gap-1">
        <div className={LABEL_CLASS}>Temperature / Dewpoint</div>
        <div className="flex flex-col items-center gap-1">
          <Thermometer className={ICON_CLASS} />
          <div className={VALUE_CLASS}>
            {temperature != null
              ? `${temperature} °C${dewPoint != null ? ` / ${dewPoint} °C` : ""}`
              : "—"}
          </div>
        </div>
      </div>
      <div className="flex flex-col items-center gap-1">
        <div className={LABEL_CLASS}>QNH</div>
        <div className="flex flex-col items-center gap-1">
          <Gauge className={ICON_CLASS} />
          <div className={VALUE_CLASS}>{qnh != null ? (qnhUnit === "hPa" ? `${qnh} hPa` : `${qnh} inHg`) : "—"}</div>
        </div>
      </div>
    </div>
  );
}

function conditionLabel(condition: MetarDecoded["condition"]): string {
  switch (condition) {
    case "clear": return "Clear";
    case "few": return "Few";
    case "sct": return "Scattered";
    case "bkn": return "Broken";
    case "ovc": return "Overcast";
    case "fg": return "Fog";
    case "precip": return "Precipitation";
    case "thunderstorm": return "Thunderstorm";
    default: return condition;
  }
}

/** Build a short, spoken-style readout from decoded METAR for easy readback. */
function buildReadout(decoded: MetarDecoded): string {
  const { flightCategoryLabel, condition, windSpeedKts, windDegrees, windDirection, visibilityDisplay, ceilingFt, temperature, dewPoint, qnh, qnhUnit } = decoded;
  const parts: string[] = [];

  parts.push(flightCategoryLabel.split(" — ")[0]);
  parts.push(conditionLabel(condition) + ".");
  if (windSpeedKts != null && windDegrees != null) {
    parts.push(`Wind ${windDegrees} degrees, ${windSpeedKts} knots.`);
  } else if (windDirection === "VRB" && windSpeedKts != null) {
    parts.push(`Wind variable, ${windSpeedKts} knots.`);
  } else {
    parts.push("Wind not reported.");
  }
  const visRead = visibilityDisplay === "—" ? "not reported" : visibilityDisplay.replace("≥10 km", "10 kilometres or more").replace(" km", " kilometres");
  parts.push(`Visibility ${visRead}.`);
  parts.push(ceilingFt != null ? `Ceiling ${ceilingFt.toLocaleString()} feet.` : "Ceiling not reported.");
  if (temperature != null) {
    parts.push(dewPoint != null ? `Temperature ${temperature}, dewpoint ${dewPoint}.` : `Temperature ${temperature}.`);
  } else {
    parts.push("Temperature not reported.");
  }
  parts.push(qnh != null ? `QNH ${qnh} ${qnhUnit === "hPa" ? "hectopascals" : "inches mercury"}.` : "QNH not reported.");

  return parts.join(" ");
}

export default function ATIS({ atisCode }: { atisCode?: string | null }) {
  const airport = useAirport();
  const metar = useMetar();
  const decoded = decodeMetar(metar ?? undefined);

  return (
    <Dialog>
      <DialogTrigger asChild>
        <button className={`${CLS_CMDBTN} !w-auto px-5`}>ATIS</button>
      </DialogTrigger>
      <DialogContent className={CLS_DIALOG}>
        <DialogHeader>
          <DialogTitle className="text-primary font-semibold text-xl">
            METAR — {airport || "EKCH"}
          </DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-5">
          <DecodedGrid decoded={decoded} />
          <div className="rounded border-2 border-gray-300 bg-gray-50 p-4">
            <div className="text-gray-600 font-medium text-sm mb-2">Readout</div>
            <p className="text-xl font-medium leading-relaxed text-black">
              {decoded.parsed ? buildReadout(decoded) : "No METAR available."}
            </p>
            {metar && (
              <pre className="mt-3 font-mono text-xs text-gray-500 whitespace-pre-wrap break-words pt-3 border-t border-gray-200">
                {metar}
              </pre>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
