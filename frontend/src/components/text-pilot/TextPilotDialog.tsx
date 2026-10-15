import { useState, type CSSProperties } from "react";
import { ActionType, type FrontendSendEvent } from "@/api/models";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { useWebSocketStore } from "@/store/store-hooks";
import { clearancePreview, defaultTextSelections, stationController, textPilotPhrases, togglePhraseText, type TextPilotDraft, type Phrase } from "@/lib/text-pilot";
import "./TextPilotDialog.css";
import storyboardLayouts from "./storyboard-layouts.json";

export function TextPilotDialog() {
  const draft = useWebSocketStore(s => s.textPilotDraft);
  return draft ? <Composer key={`${draft.callsign}:${draft.variant}`} draft={draft} /> : null;
}

function Composer({ draft }: { draft: TextPilotDraft }) {
  const strip = useWebSocketStore(s => s.strips.find(s => s.callsign === draft.callsign));
  const metar = useWebSocketStore(s => s.metar);
  const controllers = useWebSocketStore(s => s.controllers);
  const position = useWebSocketStore(s => s.position);
  const transfer = useWebSocketStore(s => s.stripTransfers[draft.callsign]);
  const close = useWebSocketStore(s => s.closeTextPilotMenu);
  const sendMessage = useWebSocketStore(s => s.sendFrequencyMessage);
  const markReadback = useWebSocketStore(s => s.markTextPilotReadback);
  const phrases = textPilotPhrases(draft, { runway: strip?.runway ?? "", stand: strip?.stand }, metar);
  const initial = [...defaultTextSelections(draft)];
  if (draft.variant === "runway" && strip?.runway) initial.push(`rwy:${strip.runway}`);
  const [selected, setSelected] = useState(initial);
  const [text, setText] = useState(() => draft.initialText ?? (draft.variant === "clearance" && strip ? clearancePreview(strip) : undefined) ?? initial.map(id => phrases.find(p => p.id === id)?.text).filter(Boolean).join(" "));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [remainingActions, setRemainingActions] = useState(draft.actions ?? []);
  const [actionsApplied, setActionsApplied] = useState(false);
  const controlsLocked = pending || actionsApplied;

  const toggle = (p: Phrase) => {
    if (controlsLocked) return;
    const removing = isSelected(p);
    if (draft.variant === "push" && /^(limit$|release:|number:|face:)/.test(p.id)) {
      const tokenFor = (ids: string[]) => {
        if (ids.includes("limit")) return phrases.find(p => p.id === "limit")?.text ?? "";
        const face = ids.find(id => id.startsWith("face:"))?.slice(5);
        if (face) return `Face ${face}.`;
        const point = (ids.find(id => id.startsWith("release:"))?.slice(8) ?? "") + (ids.find(id => id.startsWith("number:"))?.slice(7) ?? "");
        return point ? `Release point ${point}.` : "";
      };
      let next = [...selected];
      if (next.includes("limit") && /^(release:|number:)/.test(p.id)) {
        const parts = draft.point?.match(/^([A-Z]+)([0-9]+)?$/);
        if (parts && !["N", "E", "W", "S"].includes(parts[1])) {
          next.push(`release:${parts[1]}`);
          if (parts[2]) next.push(`number:${parts[2]}`);
        }
      }
      if (/^(limit$|face:)/.test(p.id)) next = next.filter(id => !/^(limit$|release:|number:|face:)/.test(id));
      else {
        const category = p.id.split(":")[0] + ":";
        next = next.filter(id => id !== "limit" && !id.startsWith("face:") && !id.startsWith(category));
      }
      if (!removing) next.push(p.id);
      setText(togglePhraseText(togglePhraseText(text, tokenFor(selected), true), tokenFor(next), false));
      setSelected(next);
      return;
    }
    let nextText = text;
    let next = [...selected];
    // A contact instruction can name exactly one target. Changing the station
    // removes only its generated phrase, retaining other manual text.
    if (!removing && p.id.startsWith("freq:")) {
      for (const id of next.filter(id => id.startsWith("freq:"))) {
        const previous = phrases.find(p => p.id === id);
        if (previous) nextText = togglePhraseText(nextText, previous.text, true);
      }
      next = next.filter(id => !id.startsWith("freq:"));
      next = next.filter(id => id !== "remain");
      nextText = togglePhraseText(nextText, phrases.find(p => p.id === "remain")?.text ?? "", true);
    }
    if (!removing && p.id === "remain") {
      for (const id of next.filter(id => id.startsWith("freq:"))) {
        nextText = togglePhraseText(nextText, phrases.find(p => p.id === id)?.text ?? "", true);
      }
      next = next.filter(id => !id.startsWith("freq:"));
    }
    if (removing) nextText = togglePhraseText(nextText, p.text, true);
    else {
      const suffix = next.map(id => phrases.find(p => p.id === id)).reverse().find(p => p?.id === "limit" || p?.id === "stand" || p?.id.startsWith("hp:") || p?.id.startsWith("hold-") && p.id !== "hold-short");
      const at = suffix ? nextText.lastIndexOf(suffix.text) : -1;
      if (p.id === "taxi") nextText = `${p.text} ${nextText}`.trim();
      else if (at >= 0 && p.id !== "limit" && p.id !== "stand" && !p.id.startsWith("hp:")) nextText = `${nextText.slice(0,at).trim()} ${p.text} ${nextText.slice(at)}`.trim();
      else nextText = togglePhraseText(nextText, p.text, false);
    }
    setText(nextText);
    setSelected(removing ? next.filter(id => id !== p.id) : [...next, p.id]);
  };

  const send = async () => {
    if (pending || !strip || !text.trim()) return;
    let actions: FrontendSendEvent[] = remainingActions;
    if (draft.variant === "push") {
      const face = selected.find(id => id.startsWith("face:"))?.slice(5);
      const point = selected.includes("limit") ? draft.point ?? "" : face
        ? ({ North: "N", East: "E", West: "W", South: "S" } as Record<string, string>)[face] ?? ""
        : (selected.find(id => id.startsWith("release:"))?.slice(8) ?? "") + (selected.find(id => id.startsWith("number:"))?.slice(7) ?? "");
      // Resolve only actions still pending, so a retry never replays an applied update.
      actions = actions.map(action => action.type === ActionType.FrontendReleasePoint && action.callsign === draft.callsign
        ? { ...action, release_point: point } : action);
    }
    if (draft.variant === "si" && !actionsApplied) {
      const frequency = selected.find(id => id.startsWith("freq:"))?.slice(5);
      if (strip.owner === position && !transfer?.to && frequency) {
        if (draft.transferAllowed && !draft.transferAllowed()) { setError("Startup transfer is only available within the TSAT window"); return; }
        const target = stationController(frequency, controllers);
        if (!target) { setError("The selected station is not available for transfer"); return; }
        actions = target.position === position ? [] : [{ type: ActionType.FrontendCoordinationTransferRequest, callsign: draft.callsign, to: target.position, ...(draft.startRequestTransfer ? { start_req_transfer: true } : {}) }];
      } else if (transfer?.from === position && selected.includes("remain")) {
        actions = [{ type: ActionType.FrontendCoordinationCancelTransferRequest, callsign: draft.callsign }];
      }
    }
    setPending(true); setError("");
    const result = await sendMessage(draft.callsign, text, actions, draft.variant === "clearance" && actions.length > 0);
    setPending(false);
    setRemainingActions(actions.slice(result.applied_actions));
    if (result.applied_actions > 0) setActionsApplied(true);
    if (!result.submitted) { setError(`${result.error ?? "Text was not submitted"}${actionsApplied || result.applied_actions > 0 ? ". Strip changes were accepted; retry this message or close and reopen to change instructions." : ""}`); return; }
    if (draft.readbackKey && selected.includes("readback")) markReadback(draft.readbackKey);
    draft.onSubmitted?.();
    close();
  };

  const isSelected = (p: Phrase) => selected.includes(p.id) || draft.variant === "push" && selected.includes("limit") && (
    p.id === `release:${draft.point?.match(/^([A-Z]+)([0-9]+)?$/)?.[1]}` ||
    p.id === `number:${draft.point?.match(/^([A-Z]+)([0-9]+)?$/)?.[2]}` ||
    p.id === `face:${({ N: "North", E: "East", W: "West", S: "South" } as Record<string, string>)[draft.point ?? ""]}`
  );
  const button = (p: Phrase, style?: CSSProperties, visualLabel = p.label) => (
    <button key={p.id} type="button" className={`fs-text-pilot-button fs-text-pilot-${p.tone} ${p.id.startsWith("freq:") ? "fs-text-pilot-station" : ""}`} style={{ ...style, ...(isSelected(p) ? { background: "#70ed45", color: "#000" } : {}) }}
      aria-pressed={isSelected(p)} disabled={controlsLocked || p.disabled} onClick={() => toggle(p)}>
      {p.id.startsWith("freq:") ? <><span className="fs-text-pilot-station-name">{p.label.split("\n")[0]}</span><span className="fs-text-pilot-station-frequency">{p.label.split("\n")[1]}</span></> : p.id === "wind" && p.text ? p.text : visualLabel}
    </button>
  );
  const runway = draft.variant === "runway";
  const si = draft.variant === "si";
  type LayoutBox = { label: string; left: number; top: number; width: number; height: number; fontSize: number; background?: string; color?: string; fontWeight?: number; phraseId?: string };
  const storyboard = storyboardLayouts[draft.variant as keyof typeof storyboardLayouts];
  if (storyboard) {
    const placement = (box: { left: number; top: number; width: number; height: number; fontSize?: number; background?: string; color?: string; fontWeight?: number }): CSSProperties => ({
      position: "absolute", left: `${box.left/storyboard.width*100}%`, top: `${box.top/storyboard.height*100}%`,
      width: `${box.width/storyboard.width*100}%`, height: `${box.height/storyboard.height*100}%`, fontSize: box.fontSize ? `${box.fontSize/storyboard.width*100}cqw` : undefined, background: box.background, color: box.color, fontWeight: box.fontWeight,
    });
    const normalized = (value: string) => value.replace(/[\s.,]/g, "").toUpperCase();
    return <Dialog open onOpenChange={open => { if (!open && !pending) close(); }}>
      <DialogContent className={`fs-text-pilot-dialog fs-text-pilot-storyboard fs-text-pilot-dialog-${draft.variant} [&>button]:hidden`} style={{ width: `min(94vw, ${90*storyboard.width/storyboard.height}dvh)` }} onEscapeKeyDown={e => { if (pending) e.preventDefault(); }}>
        <div className="fs-text-pilot-stage" style={{ aspectRatio: `${storyboard.width}/${storyboard.height}` }}>
          <div className="fs-text-pilot-frame" style={placement(storyboard.frame)} />
          {"dividers" in storyboard && storyboard.dividers.map((line, index) => <div key={`divider:${index}`} className="fs-text-pilot-divider" aria-hidden style={{ ...placement(line), background: "#000" }} />)}
          {!storyboard.boxes.some(box => box.label === "CALLSIGN") && <DialogTitle className="fs-text-pilot-stage-title">{draft.callsign} text message</DialogTitle>}
          {storyboard.boxes.map((box, index) => {
            const style = placement(box as LayoutBox);
            if (box.label === "CALLSIGN") return <DialogTitle key={index} style={style} className="fs-text-pilot-stage-title">{draft.callsign}{draft.variant === "tower-out" ? <span className="sr-only"> text message</span> : " text message"}</DialogTitle>;
            if (box.label === "text field" || box.label.startsWith("PUSH AND START APPROVED,")) return <textarea key={index} style={style} className="fs-text-pilot-stage-editor" aria-label="Text message" autoFocus value={text} disabled={controlsLocked} onChange={e => setText(e.target.value.replace(/[\r\n]/g," "))} />;
            if (box.label === "ERASE") return <button key={index} style={style} className="fs-text-pilot-erase" disabled={controlsLocked} onClick={() => { setText(""); setSelected([]); }}>ERASE</button>;
            if (box.label === "SEND" || box.label === "ESC") return <button key={index} style={style} className="fs-text-pilot-stage-action" disabled={pending || box.label === "SEND" && (!strip || !text.trim())} onClick={box.label === "SEND" ? () => void send() : close}>{box.label === "SEND" && pending ? "SENDING…" : box.label}</button>;
            if (box.label === "------>") return <span key={index} style={style} className="fs-text-pilot-stage-group" aria-hidden>{"------>"}</span>;
            if (["RELEASE POINT", "FACE TO THE", "CONTACT"].includes(box.label)) {
              const contactSelected = box.label === "CONTACT" && selected.some(id => id.startsWith("freq:"));
              return <div key={index} style={{ ...style, ...(contactSelected ? { background: "#70ed45", color: "#000" } : {}) }} className="fs-text-pilot-stage-group" data-selected={contactSelected}>{box.label}</div>;
            }
            if (box.label === "ASSIGNED STAND" && !phrases.some(p => p.id === "stand")) return <button key={index} style={style} className="fs-text-pilot-button fs-text-pilot-point" disabled>ASSIGNED STAND</button>;
            const p = "phraseId" in box ? phrases.find(p => p.id === box.phraseId) : box.label === "ASSIGNED STAND" ? phrases.find(p => p.id === "stand") : phrases.find(p => normalized(p.label) === normalized(box.label));
            return p ? <span key={index}>{button(p, style, p.id === "limit" || p.id === "stand" ? p.label : box.label)}</span> : null;
          })}
          {error && <p className="fs-text-pilot-stage-error" role="alert">{error}</p>}
        </div>
      </DialogContent>
    </Dialog>;
  }
  return (
    <Dialog open onOpenChange={open => { if (!open && !pending) close(); }}>
      <DialogContent className={`fs-text-pilot-dialog fs-text-pilot-dialog-${draft.variant} [&>button]:hidden`} onEscapeKeyDown={e => { if (pending) e.preventDefault(); }}>
        <div className="fs-text-pilot-outline">
          <DialogTitle>{draft.callsign} text message</DialogTitle>
          <div className="fs-text-pilot-editor">
            <textarea aria-label="Text message" autoFocus value={text} disabled={controlsLocked} onChange={e => setText(e.target.value.replace(/[\r\n]/g," "))} />
            <button className="fs-text-pilot-erase" disabled={controlsLocked} onClick={() => { setText(""); setSelected([]); }}>ERASE</button>
          </div>
          {si ? <div className="fs-text-pilot-si">
            <div className="fs-text-pilot-si-phrases">{phrases.filter(p => !p.id.startsWith("freq:")).map(p => button(p))}</div>
            <div className="fs-text-pilot-si-stations"><div className="fs-text-pilot-contact" aria-disabled="true" data-selected={selected.some(id => id.startsWith("freq:"))}>CONTACT</div>{phrases.filter(p => p.id.startsWith("freq:")).map(p => button(p))}</div>
          </div> : runway ? <div className="fs-text-pilot-runway">
            {phrases.filter(p => p.id === "wind").map(p => button(p))}
            <div className="fs-text-pilot-runway-instructions">{phrases.filter(p => ["continue-approach","land","takeoff","lineup"].includes(p.id)).map(p => button(p))}</div>
            <div className="fs-text-pilot-runways">{phrases.filter(p => p.id.startsWith("rwy:")).map(p => button(p))}</div>
            <div className="fs-text-pilot-runway-extras">{phrases.filter(p => ["long-rollout","expedite-vacating","wake","rolling"].includes(p.id)).map(p => button(p))}</div>
          </div> : <div className="fs-text-pilot-phrases">{phrases.map(p => button(p))}</div>}
          {error && <p className="fs-text-pilot-error" role="alert">{error}</p>}
          {!strip && <p role="alert">Aircraft is no longer available</p>}
          <div className="fs-text-pilot-actions">
            <button disabled={pending} onClick={close}>ESC</button>
            <button disabled={pending || !strip || !text.trim()} onClick={() => void send()}>{pending ? "SENDING…" : "SEND"}</button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
