import { isTextPilot, textTaxiActions } from "@/lib/text-pilot";
import { controllerOwnsTowerSectors, useWebSocketStore, useRunwaySetup, useApronOnline, useTwrOnline, useIsTwr, useMyPosition, useStrip } from "@/store/store-hooks";
import { COORDINATION_WITH_APRON_TAXI_MAP_POINTS, COORDINATION_WITH_TWR_TAXI_MAP_POINTS, HOLDING_POINT_RUNWAYS, TAXI_MAP_POINTS } from "@/config/ekch";
import type { ClickPoint, VisibilityContext } from "@/config/ekch";
import { MAP_BTN_BASE, MAP_POINT_STROKE_STYLE, MapDialogShell, MapEraseControls } from "./MapDialogShell";
import { Bay } from "@/api/models";
import { isRestrictedTwrPoint } from "./aircraftMapRestrictions";


interface TaxiMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  moveOnSelect?: boolean;
}

// Sizes expressed as % of image container (1801×1013 reference) so they scale with the dialog.
const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: "3.89%",  // 70px at 1801px wide
  height: "3.95cqh", // 40px at 1013px tall
};

export function TaxiMapDialog({
  open,
  onOpenChange,
  callsign,
  moveOnSelect = false,
}: TaxiMapDialogProps) {
  const openTextPilotMenu = useWebSocketStore(s => s.openTextPilotMenu);
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const acknowledgeUnexpectedChange = useWebSocketStore((s) => s.acknowledgeUnexpectedChange);
  const move = useWebSocketStore((s) => s.move);
  const runwaySetup = useRunwaySetup();
  const apronOnline = useApronOnline();
  const twrOnline   = useTwrOnline();
  const isTwr       = useIsTwr();
  const strip = useStrip(callsign);
  const myPosition = useMyPosition();
  const isPointDisabled = (label: string, point?: Pick<ClickPoint, "type">) =>
    isRestrictedTwrPoint(label, point?.type, strip?.aircraft_type, strip?.aircraft_category);
  const disabledMessage = "Aircraft type not approved for this Taxiway";
  const layout = useWebSocketStore(s => s.displayedLayout || s.layout);
  const airport = useWebSocketStore((s) => s.airport);
  const controllers = useWebSocketStore((s) => s.controllers);

  const stripType = strip?.origin === airport ? "dep"
    : strip?.destination === airport ? "arr"
    : undefined;

  const visibilityContext: VisibilityContext = {
    dep: runwaySetup.departure,
    arr: runwaySetup.arrival,
    stripType,
    apronOnline,
    twrOnline,
    isTwr: isTwr ?? false,
  };

  const shouldAcknowledgeReleasePoint =
    strip?.unexpected_change_fields?.includes("release_point") && !!myPosition && strip.owner === myPosition;

  const isNotOwner = !!myPosition && !!strip?.owner && strip.owner !== myPosition;
  const ownerController = controllers.find((c) => c.position === strip?.owner);
  const coordinationPoints =
    controllerOwnsTowerSectors(ownerController)
      ? COORDINATION_WITH_TWR_TAXI_MAP_POINTS
      : COORDINATION_WITH_APRON_TAXI_MAP_POINTS;

  const getConfiguredPoint = (label: string) => TAXI_MAP_POINTS.find((point) =>
    (typeof point.label === "string" ? point.label : point.label(visibilityContext)) === label
  );
  const isConfiguredHoldingPoint = (label: string) =>
    HOLDING_POINT_RUNWAYS.some((runway) =>
      runway.points.some((point) => point.label === label)
    )
    || TAXI_MAP_POINTS.some((point) =>
      point.type === "hp"
      && (typeof point.label === "string" ? point.label : point.label(visibilityContext)) === label
    );
  const getConfiguredPointType = (label: string) =>
    getConfiguredPoint(label)?.type ?? (isConfiguredHoldingPoint(label) ? "hp" : undefined);
  const canSelectManualPoint = (label: string) =>
    !isPointDisabled(label, { type: getConfiguredPointType(label) });

  const handleSelect = (label: string, point?: ClickPoint, manuallyEntered = false) => {
    if (manuallyEntered ? !canSelectManualPoint(label) : isPointDisabled(label, point)) return;
    if (isTextPilot(strip)) {
      openTextPilotMenu({ callsign, variant: strip?.destination === airport && strip?.origin !== airport ? "tower-in" : "tower-out", point: label, splitTowerDefaults: ["GEGW", "TWTE"].includes(layout), actions: textTaxiActions(callsign, label, moveOnSelect ? Bay.TaxiLwr : undefined, shouldAcknowledgeReleasePoint), onSubmitted: () => onOpenChange(false) });
      return;
    }
    if (label !== strip?.release_point) {
      setReleasePoint(callsign, label);
    }
    if (moveOnSelect) {
      move(callsign, Bay.TaxiLwr);
    }
    if (shouldAcknowledgeReleasePoint) {
      acknowledgeUnexpectedChange(callsign, "release_point");
    }
    onOpenChange(false);
  };

  const handleErase = () => {
    if (strip?.release_point) {
      setReleasePoint(callsign, "");
    }
    if (shouldAcknowledgeReleasePoint) {
      acknowledgeUnexpectedChange(callsign, "release_point");
    }
    onOpenChange(false);
  };

  return (
    <MapDialogShell
      open={open}
      onOpenChange={onOpenChange}
      title="Select Taxi Route"
      imageSrc="/taxi_map.webp"
      imageAlt="TWR taxi map"
      imgWidth={1801}
      imgHeight={1013}
      points={isNotOwner ? coordinationPoints : TAXI_MAP_POINTS}
      pointStyle={MAP_POINT_STROKE_STYLE}
      btnStyle={BTN_STYLE}
      onSelect={handleSelect}
      isPointDisabled={isPointDisabled}
      disabledPointMessage={disabledMessage}
      visibilityContext={visibilityContext}
      selectedPoint={strip?.release_point}
      >
        {/* Controls panel — bottom-left */}
        <div
          style={{
            position: "absolute",
            bottom: "5%",
            left: "2%",
            zIndex: 20,
            display: "flex",
            flexDirection: "column",
            width: "fit-content",
            gap: "0.39cqh",
          }}
        >
          <MapEraseControls onOk={label => handleSelect(label, undefined, true)} onErase={handleErase} onClose={() => onOpenChange(false)} btnStyle={BTN_STYLE}
            canSelect={canSelectManualPoint} disabledMessage={disabledMessage} />
        </div>
      </MapDialogShell>
  );
}
