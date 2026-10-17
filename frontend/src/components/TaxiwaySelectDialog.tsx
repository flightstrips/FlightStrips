import { useState } from "react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { MENU_BUTTON_BEVEL } from "@/components/ui/menuControlStyle";
import { EST_RAISED_EDGE } from "@/components/est/bevel";
import { scalePx } from "@/lib/viewportScale";

const TAXIWAYS = [
  ["K", 56, 75, 40, 27], ["J", 104, 75, 40, 27], ["L", 152, 75, 40, 27],
  ["M", 200, 75, 40, 27], ["P", 248, 75, 40, 27], ["Q", 296, 75, 40, 27],
  ["R", 387, 75, 40, 27], ["S", 435, 75, 40, 27], ["W", 483, 75, 84, 27],
  ["T", 575, 75, 40, 27], ["I", 623, 75, 85, 27], ["G", 716, 75, 86, 27],
  ["K2", 56, 113, 40, 66], ["K3", 104, 113, 40, 66],
  ["Z", 152, 113, 109, 27], ["Y", 269, 113, 110, 27], ["V", 387, 113, 228, 27],
  ["G2", 623, 113, 85, 27], ["G1", 716, 113, 86, 27],
  ["A", 154, 152, 85, 27], ["F", 247, 152, 85, 27], ["D", 340, 152, 85, 27],
  ["B", 433, 152, 86, 27],
  ["V2", 527, 152, 40, 27], ["V1", 575, 152, 40, 27], ["K1", 623, 152, 40, 27],
  ["F1", 671, 152, 40, 27], ["G2", 719, 152, 40, 27], ["G1", 767, 152, 40, 27],
  ...["A1", "A2", "A3", "A4", "A5"].map((label, i) => [label, 56 + i * 48, 186, 40, 27] as const),
  ...["A6", "A7", "A8", "A9", "A10"].map((label, i) => [label, 56 + i * 48, 221, 40, 27] as const),
  ["E1", 311, 186, 40, 27], ["E2", 359, 186, 40, 27],
  ["E3", 311, 221, 40, 27], ["E4", 359, 221, 40, 27],
  ["B1", 435, 186, 40, 27], ["B2", 483, 186, 40, 27], ["B3", 531, 186, 40, 27],
  ["B4", 435, 221, 40, 27], ["B5", 483, 221, 40, 27],
] as const;

interface Props {
  onClose: () => void;
  onSelect: (taxiway: string) => void;
}

export function TaxiwaySelectDialog({ onClose, onSelect }: Props) {
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const selected = selectedIndex === null ? "" : TAXIWAYS[selectedIndex][0];
  return (
    <Dialog open onOpenChange={open => { if (!open) onClose(); }}>
      <DialogContent className="block max-w-none gap-0 p-0 [&>button:last-child]:hidden"
        style={{ ...EST_RAISED_EDGE, width: scalePx(858), height: scalePx(466), background: "#E4E4E4", fontFamily: "var(--font-bay)" }}>
        <DialogTitle className="sr-only">Select taxiway</DialogTitle>
        <div className="absolute border border-[#666]" style={{ left: scalePx(45), top: scalePx(45), right: scalePx(40), bottom: scalePx(20) }} />
        {TAXIWAYS.map(([label, left, top, width, height], index) => {
          const dark = top >= 186 || (top === 152 && left >= 527);
          return (
            <button key={`${label}-${index}`} type="button" aria-pressed={selectedIndex === index}
              className="absolute flex items-center justify-center active:brightness-90"
              style={{ ...MENU_BUTTON_BEVEL, left: scalePx(left), top: scalePx(top), width: scalePx(width), height: scalePx(height),
                background: selectedIndex === index ? "#00FF26" : dark ? "#96917B" : "#D6D6D6", color: dark && selectedIndex !== index ? "white" : "black", fontSize: scalePx(14), cursor: "pointer" }}
              onClick={() => setSelectedIndex(index)}>{label}</button>
          );
        })}
        {["ESC", "OK"].map((label, index) => (
          <button key={label} type="button" disabled={label === "OK" && !selected}
            className="absolute font-bold active:brightness-90 disabled:opacity-50"
            style={{ ...MENU_BUTTON_BEVEL, left: scalePx(304 + index * 132), top: scalePx(383), width: scalePx(122), height: scalePx(44), background: "#3F3F3F", color: "white", fontSize: scalePx(20) }}
            onClick={() => { if (label === "OK") onSelect(selected); onClose(); }}>{label}</button>
        ))}
      </DialogContent>
    </Dialog>
  );
}
