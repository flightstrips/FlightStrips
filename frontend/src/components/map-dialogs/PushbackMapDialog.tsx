import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { isTextPilot, textTaxiActions } from "@/lib/text-pilot";
import { useStrip, useWebSocketStore } from "@/store/store-hooks";
import { ActionType, Bay } from "@/api/models";
import { RELEASE_POINTS } from "@/config/ekch";
import { MAP_BTN_BASE, MapDialogShell, MapEraseControls } from "./MapDialogShell";

interface PushbackMapDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  initialReleasePoint?: string;
  onStripMoved?: () => void;
}

// Sizes expressed in container-query units (1920×768 reference) so they scale with the dialog.
const BTN_STYLE: React.CSSProperties = {
  ...MAP_BTN_BASE,
  width: "2.86cqw",    // 55px at 1920px wide
  height: "3.91cqh",   // 30px at 768px tall
  fontSize: "2.34cqh", // 18px at 768px tall
};

export function PushbackMapDialog({ open, onOpenChange, callsign, initialReleasePoint, onStripMoved }: PushbackMapDialogProps) {
  const [checking, setChecking] = useState(false);
  const attempt = useRef(0);
  const cancelPreparation = useCallback(() => {
    attempt.current++;
    setChecking(false);
  }, []);
  useEffect(() => {
    if (!open) cancelPreparation();
    return cancelPreparation;
  }, [open, cancelPreparation]);
  const sendFrequencyMessage = useWebSocketStore(s => s.sendFrequencyMessage);
  const openValidationDialog = useWebSocketStore(s => s.openValidationDialog);
  const handleOpenChange = (next: boolean) => {
    if (!next) cancelPreparation();
    onOpenChange(next);
  };
  const openTextPilotMenu = useWebSocketStore(s => s.openTextPilotMenu);
  const setReleasePoint = useWebSocketStore((s) => s.setReleasePoint);
  const strip = useStrip(callsign);
  const move = useWebSocketStore((s) => s.move);

  const handleSelect = async (label: string) => {
    if (checking) return;
    if (isTextPilot(strip)) {
      const current = ++attempt.current;
      setChecking(true);
      const result = await sendFrequencyMessage(callsign, "", [{ type: ActionType.FrontendMove, callsign, bay: Bay.Push }], false, true);
      if (attempt.current !== current || !open) return;
      setChecking(false);
      if (!result.prepared) {
        if (result.validation_blocked) {
          handleOpenChange(false);
          openValidationDialog(callsign);
        }
        else toast.error(result.error ?? "Pushback validation was not accepted");
        return;
      }
      openTextPilotMenu({ callsign, variant: "push", point: label, actions: textTaxiActions(callsign, label, Bay.Push), onSubmitted: () => { onOpenChange(false); onStripMoved?.(); } });
      return;
    }
    setReleasePoint(callsign, label);
    move(callsign, Bay.Push);
    onOpenChange(false);
    onStripMoved?.();
  };

  return (
    <MapDialogShell
      open={open}
      onOpenChange={handleOpenChange}
      title="Select Release Point"
      imageSrc="/apron_push.webp"
      imageAlt="Apron pushback map"
      imgWidth={1920}
      imgHeight={768}
      points={RELEASE_POINTS}
      btnStyle={BTN_STYLE}
      onSelect={handleSelect}
      selectedPoint={initialReleasePoint}
    >
      {/* Controls panel — bottom-left */}
      <div
        style={{
          position: "absolute",
          left: "2.5%",
          top: "70%",
          zIndex: 20,
          display: "flex",
          flexDirection: "row",
          alignItems: "flex-start",
          gap: "0.63cqw",
        }}
      >
        {checking && <span role="status">Checking pushback timing...</span>}
        {/* Direction quick-select */}
        <div style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: "0.52cqh" }}>
          <button onClick={() => handleSelect("N")} style={BTN_STYLE}>N</button>
          <div style={{ display: "flex", gap: "0.52cqh" }}>
            <button onClick={() => handleSelect("W")} style={BTN_STYLE}>W</button>
            <button onClick={() => handleSelect("E")} style={BTN_STYLE}>E</button>
          </div>
          <button onClick={() => handleSelect("S")} style={BTN_STYLE}>S</button>
        </div>

        {/* ERASE / input / OK — offset down to align with W/E row */}
        <div style={{ display: "flex", flexDirection: "column", gap: "0.52cqh", marginTop: "4.43cqh", width: "fit-content" }}>
          <MapEraseControls onOk={handleSelect} onClose={() => handleOpenChange(false)} btnStyle={BTN_STYLE} maxLength={4} />
        </div>
      </div>
    </MapDialogShell>
  );
}

