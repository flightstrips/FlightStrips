import { toast } from "sonner";
import { isTextPilot } from "@/lib/text-pilot";
import { useIsTwr, useStrip } from "@/store/store-hooks";
import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { ActionType } from "@/api/models";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { useWebSocketStore } from "@/store/store-hooks";
import { STAND_ASSIGNMENT_LAYOUTS, STAND_ASSIGNMENT_LETTERS as LETTERS } from "./standAssignmentLayouts";
import { deriveStandAvailability, type StandBadge } from "./standAvailability";
import StandStatusPage from "@/pages/stand-status";
import "./ArrStandDialog.css";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  currentStand?: string;
}

function position(left: number, top: number, width: number, height: number, frameWidth = 878, frameHeight = 768): CSSProperties {
  return {
    position: "absolute",
    left: `${left / frameWidth * 100}%`,
    top: `${top / frameHeight * 100}%`,
    width: `${width / frameWidth * 100}%`,
    height: `${height / frameHeight * 100}%`,
  };
}

type StandGroup = (typeof LETTERS)[number];

function defaultGroup(stand: string): StandGroup | null {
  if (stand.startsWith("HANGAR") || stand.startsWith("RUNUP")) return null;
  return LETTERS.find(letter => stand.startsWith(letter)) ?? null;
}

function CallsignBadge({ badge }: { badge: StandBadge }) {
  const badgeRef = useRef<HTMLSpanElement>(null);

  useLayoutEffect(() => {
    const container = badgeRef.current;
    if (!container) return;
    const fit = () => {
      container.style.fontSize = "";
      const styles = getComputedStyle(container);
      const fontSize = parseFloat(styles.fontSize);
      const availableWidth = container.clientWidth - parseFloat(styles.paddingLeft) - parseFloat(styles.paddingRight);
      if (availableWidth <= 0) return;
      const range = document.createRange();
      range.selectNodeContents(container);
      const textWidth = range.getBoundingClientRect().width;
      if (availableWidth > 0 && textWidth > availableWidth) {
        container.style.fontSize = `${fontSize * (availableWidth - 1) / textWidth}px`;
      }
    };
    fit();
    const observer = new ResizeObserver(fit);
    if (container.parentElement) observer.observe(container.parentElement);
    return () => observer.disconnect();
  }, [badge.label]);

  return (
    <span ref={badgeRef} title={badge.label} className={`stand-assignment-badge stand-assignment-badge-${badge.blocked ? "blocked" : "assigned"}`}>
      {badge.label}
    </span>
  );
}

function MenuButton({ children, onClick, style, selected = false, tone = "light", disabled = false, title, badges, label }: {
  children: ReactNode;
  onClick: () => void;
  style?: CSSProperties;
  selected?: boolean;
  tone?: "light" | "area" | "selector" | "dark" | "auto";
  disabled?: boolean;
  title?: string;
  badges?: StandBadge[];
  label?: string;
}) {
  return (
    <button
      type="button"
      className={`stand-assignment-button stand-assignment-button-${tone}${badges?.length ? " stand-assignment-button-with-badges" : ""}`}
      style={style}
      aria-pressed={tone !== "dark" ? selected : undefined}
      disabled={disabled}
      title={title}
      aria-label={label}
      onClick={onClick}
    >
      {children}
      {badges?.length ? (
        <span className="stand-assignment-badges">
          {badges.map(badge => (
            <CallsignBadge key={badge.label} badge={badge} />
          ))}
        </span>
      ) : null}
    </button>
  );
}

export function ArrStandDialog(props: Props) {
  return props.open ? <StandAssignmentMenu {...props} /> : null;
}

function StandAssignmentMenu({ onOpenChange, callsign, currentStand }: Props) {
  const strip = useStrip(callsign);
  const isTower = useIsTwr();
  const openTextPilotMenu = useWebSocketStore(s => s.openTextPilotMenu);
  const sendFrequencyMessage = useWebSocketStore(s => s.sendFrequencyMessage);
  const [legacyPending, setLegacyPending] = useState(false);
  const submittedStand = useRef(false);
  const satEnabled = useWebSocketStore(s => s.satEnabled);
  const sessionId = useWebSocketStore(s => s.amanSessionID);
  const assignment = useWebSocketStore(s => s.standAssignments.find(a => a.callsign === callsign));
  const assignments = useWebSocketStore(s => s.standAssignments);
  const blocks = useWebSocketStore(s => s.standBlocks);
  const strips = useWebSocketStore(s => s.strips);
  const rejection = useWebSocketStore(s => s.standActionRejection);
  const requestAutomatic = useWebSocketStore(s => s.requestAutomaticStand);
  const requestManual = useWebSocketStore(s => s.requestManualStand);
  const confirmOverride = useWebSocketStore(s => s.confirmStandOverride);
  const clearRejection = useWebSocketStore(s => s.clearStandActionRejection);
  const updateStrip = useWebSocketStore(s => s.updateStrip);
  const initialStand = (satEnabled ? assignment?.stand : undefined) ?? currentStand ?? "";
  const [manualStand, setManualStand] = useState(initialStand);
  const [graphicOpen, setGraphicOpen] = useState(false);
  const [automaticSelected, setAutomaticSelected] = useState(satEnabled);
  const [group, setGroup] = useState<StandGroup | null>(() => defaultGroup(initialStand));
  const submittedVersion = useRef<number | null>(null);
  const version = assignment?.version ?? 0;
  const relevantRejection = satEnabled && rejection?.callsign === callsign ? rejection : null;
  const unsafeManual = relevantRejection?.action === ActionType.FrontendStandAssignmentManualRequest
    && relevantRejection.code === "incompatible_or_occupied";
  const standAvailability = useMemo(
    () => deriveStandAvailability(strips, assignments, blocks, callsign),
    [assignments, blocks, callsign, strips],
  );

  useEffect(() => {
    if (submittedVersion.current !== null && assignment && assignment.version !== submittedVersion.current) {
      submittedVersion.current = null;
      onOpenChange(false);
      if (submittedStand.current && assignment.stand && isTextPilot(strip)) {
        submittedStand.current = false;
        openTextPilotMenu({ callsign, variant: isTower ? "tower-in" : "apron-in", stand: assignment.stand });
      }
    }
  }, [assignment, onOpenChange, strip, openTextPilotMenu, callsign, isTower]);

  const close = () => {
    submittedVersion.current = null;
    clearRejection();
    onOpenChange(false);
  };
  const automatic = () => {
    submittedStand.current = true;
    setAutomaticSelected(true);
    submittedVersion.current = version;
    requestAutomatic(callsign, version);
  };
  const selectManualStand = (stand: string) => {
    setAutomaticSelected(false);
    setManualStand(stand);
  };
  const selectGroup = (nextGroup: StandGroup) => {
    setAutomaticSelected(false);
    setManualStand("");
    setGroup(nextGroup);
  };
  const selectDirectStand = (stand: string) => {
    selectManualStand(stand);
    setGroup(null);
  };
  const selectAutomatic = () => {
    if (automaticSelected) automatic();
    else setAutomaticSelected(true);
  };
  const erase = () => {
    setManualStand("");
    setGroup(null);
    setAutomaticSelected(satEnabled);
  };
  const send = () => {
    if (legacyPending) return;
    submittedStand.current = true;
    const stand = manualStand.trim().toUpperCase();
    if (satEnabled && automaticSelected) {
      automatic();
    } else if (!satEnabled) {
      if (isTextPilot(strip)) {
        setLegacyPending(true);
        void sendFrequencyMessage(callsign, "", [{ type: ActionType.FrontendUpdateStripData, callsign, stand }], false, true).then(result => {
          setLegacyPending(false);
          if (!result.prepared) { toast.error(result.error ?? "Stand assignment rejected"); return; }
          close();
          if (result.stand) openTextPilotMenu({ callsign, variant: isTower ? "tower-in" : "apron-in", stand: result.stand });
        });
      } else {
        updateStrip(callsign, { stand });
        close();
      }
    } else if (stand) {
      submittedVersion.current = version;
      requestManual(callsign, stand, version);
    }
  };
  const panel = group === "C" || group === "D" ? "C+D" : group;
  const stands = panel === null ? [] : STAND_ASSIGNMENT_LAYOUTS[panel].map(([label, left, top]) => ({
      label,
      displayLabel: label,
      style: position(left - 11, top - 7, 72, 96, 774, 751),
    }));

  return (
    <Dialog open onOpenChange={nextOpen => { if (!nextOpen) close(); }}>
      {relevantRejection ? (
        <DialogContent className="stand-assignment-warning rounded-none border-black bg-[#e4e4e4] text-black [&>button]:hidden">
          <DialogTitle>STAND ASSIGNMENT</DialogTitle>
          <p className="text-center text-2xl text-red-600">
            {relevantRejection.code === "invalid_stand" ? "STAND NOT FOUND" : relevantRejection.reason}
          </p>
          <div className="flex justify-center gap-4">
            <MenuButton tone="dark" onClick={close}>ESC</MenuButton>
            {unsafeManual && <MenuButton tone="auto" onClick={automatic}>AUTO ASSIGN</MenuButton>}
            {unsafeManual && <MenuButton tone="dark" onClick={() => {
              submittedVersion.current = version;
              confirmOverride(callsign, manualStand.trim().toUpperCase(), version, relevantRejection.reason);
            }}>YES</MenuButton>}
          </div>
        </DialogContent>
      ) : (
        <DialogContent className="stand-assignment-frame block max-w-none gap-0 rounded-none border-black bg-[#e4e4e4] p-0 text-black [&>button]:hidden">
          <div className="stand-assignment-content">
            <DialogTitle className="stand-assignment-title" style={position(0, 48, 878, 22)}>STAND ASSIGNMENT</DialogTitle>
            <div className="stand-assignment-outline" style={position(14, 72, 815, 677)} />
            {["RI", "RII", "RIII"].map((stand, index) => (
              <MenuButton key={stand} label={stand} badges={standAvailability[stand]} tone="area" selected={!automaticSelected && manualStand === stand} style={position(39 + index * 95, 87, 86, 48)} onClick={() => selectDirectStand(stand)}>{stand}</MenuButton>
            ))}
            {["W1", "SAS", "SOUTH", "WEST"].map((stand, index) => (
              <MenuButton key={stand} label={stand} badges={standAvailability[stand]} tone="area" selected={!automaticSelected && manualStand === stand} style={position(38, 147 + index * 57, 86, 48)} onClick={() => selectDirectStand(stand)}>{stand}</MenuButton>
            ))}
            <MenuButton label="HANGAR" badges={standAvailability.HANGAR} tone="area" selected={!automaticSelected && manualStand === "HANGAR"} style={position(38, 375, 86, 48)} onClick={() => selectDirectStand("HANGAR")}>HANGAR</MenuButton>
            {LETTERS.map((letter, index) => (
              <MenuButton key={letter} tone="selector" selected={!automaticSelected && group === letter} style={position(142, 153 + index * 57, 73, 48)} onClick={() => selectGroup(letter)}>{letter}</MenuButton>
            ))}
            <MenuButton tone="dark" style={position(519, 87, 127, 47)} onClick={erase}>ERASE</MenuButton>
            <input
              className="stand-assignment-input"
              style={position(666, 87, 127, 47)}
              aria-label="Manual stand"
              placeholder="e.g. C39"
              value={manualStand}
              maxLength={8}
              onChange={e => selectManualStand(e.target.value.toUpperCase())}
              onKeyDown={e => { if (e.key === "Enter") send(); }}
              autoFocus
            />
            <div className="stand-assignment-panel" role="group" aria-label={panel ? `${panel} stands` : "Stand selection"} style={position(272, 153, 520, 505)}>
              {panel === "E" && (
                <div
                  className="stand-assignment-pier"
                  aria-hidden="true"
                  style={position(188 - 11, 25 - 7, 31, 422, 774, 751)}
                />
              )}
              {stands.map(stand => (
                <MenuButton key={stand.label} label={stand.label} badges={standAvailability[stand.label]} selected={!automaticSelected && manualStand === stand.label} style={stand.style} onClick={() => selectManualStand(stand.label)}>{stand.displayLabel}</MenuButton>
              ))}
            </div>
            <MenuButton tone="dark" style={position(272, 683, 162, 47)} onClick={close}>ESC</MenuButton>
            <MenuButton tone="dark" style={position(38, 683, 177, 47)} onClick={() => setGraphicOpen(true)}>GRAPHIC</MenuButton>
            <MenuButton tone="auto" selected={automaticSelected} style={position(448, 683, 162, 47)} onClick={selectAutomatic} disabled={!satEnabled} title={!satEnabled ? "Automatic stand assignment is not enabled for this session" : undefined}>AUTO ASSIGN</MenuButton>
            <MenuButton tone="dark" style={position(627, 683, 166, 47)} onClick={send} disabled={legacyPending || (satEnabled && !automaticSelected && !manualStand.trim())}>
              {isTextPilot(strip) ? "SEND REQ" : "ASSIGN"}
              {" "}
              {(automaticSelected || manualStand.trim()) && (
                <span style={{ fontSize: "0.75em", marginLeft: "0.35em" }}>
                  ({automaticSelected ? "AUTO" : manualStand.trim().toUpperCase()})
                </span>
              )}
            </MenuButton>
          </div>
        </DialogContent>
      )}
      <Dialog open={graphicOpen} onOpenChange={setGraphicOpen}>
        {graphicOpen && (
          <DialogContent className="block w-[min(640px,calc(100vw-32px))] max-h-[calc(100dvh-32px)] overflow-y-auto border-[#777] bg-[#3f3f3f] p-0 text-white">
            <div className="flex items-center justify-between bg-[#272727] px-3 py-2">
              <DialogTitle className="text-sm">STAND GRAPHIC</DialogTitle>
              <button type="button" className="border border-[#777] px-3 py-1 text-xs text-white" onClick={() => setGraphicOpen(false)}>BACK</button>
            </div>
            <StandStatusPage compactSessionId={sessionId ?? null} initialStand={manualStand.trim().toUpperCase()} />
          </DialogContent>
        )}
      </Dialog>
    </Dialog>
  );
}
