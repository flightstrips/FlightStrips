import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";

export type RunwayStatus = "OPEN" | "LOW_VIS" | "CLOSED";

interface Props {
  pair: string;
  open: boolean;
  onClose: () => void;
  onSelect: (status: RunwayStatus) => void;
}

// All sizes derived from the SVG canvas (2560×1440 base):
//   horizontal → / 2560 * 100  = vw
//   vertical   → / 1440 * 100  = vh
//
// SVG card: 269×455px  →  10.5vw × 31.6vh
// Border rect margins: 17px → 0.664vw each side
// Options inside border: pt 31px/2.15vh, gap 18px/1.25vh, pb 36px/2.5vh
// OK gap/padding: mt 19px/1.32vh, pb 30px/2.08vh
// Button size: 164×70px → 6.4vw × 4.86vh  /  125×70px → 4.88vw × 4.86vh
// Title font: 20px → 0.78vw  |  Button font: 32px → 1.25vw

export default function RunwayStsDialog({ open, onClose, onSelect }: Props) {
  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent
        className="p-0 bg-[#b3b3b3] border border-black shadow-[0_4px_4px_rgba(0,0,0,0.25)] [&>button.absolute]:hidden"
        style={{ width: "10.5vw", maxWidth: "10.5vw" }}
      >
        <VisuallyHidden.Root>
          <DialogTitle>Runway Status</DialogTitle>
        </VisuallyHidden.Root>

        {/*
          Top spacing — gives vertical room so the border rect's title overlay
          sits comfortably inside the card. ~half the title line-height above the border.
          SVG: border rect starts at y=23 out of 455px total → ~1.6vh from top.
        */}
        <div style={{ paddingTop: "1.6vh" }}>

          {/*
            Options border box — full border (all 4 sides) so it connects all the way around.
            The "RWY STS" title is absolutely positioned on the top border with a matching
            background colour, masking the border behind it (same as an HTML <fieldset>).
          */}
          <div
            className="relative border border-black"
            style={{ marginLeft: "0.664vw", marginRight: "0.664vw" }}
          >
            {/* Title centred on the top border */}
            <div
              className="absolute inset-x-0 flex justify-center"
              style={{ top: "-0.39vw" }}
            >
              <span
                className="bg-[#b3b3b3] text-black font-light leading-none select-none"
                style={{ fontSize: "0.78vw", paddingLeft: "0.35vw", paddingRight: "0.35vw" }}
              >
                RWY STS
              </span>
            </div>

            {/* Option buttons */}
            <div
              className="flex flex-col items-center"
              style={{ paddingTop: "2.15vh", gap: "1.25vh", paddingBottom: "2.5vh" }}
            >
              <button
                onClick={() => { onSelect("OPEN"); onClose(); }}
                className="flex items-center justify-center bg-[#212121] text-white font-semibold"
                style={{ width: "6.4vw", height: "4.86vh", fontSize: "1.25vw" }}
              >
                OPEN
              </button>

              <button
                onClick={() => { onSelect("LOW_VIS"); onClose(); }}
                className="flex items-center justify-center bg-[#DD6A12] text-black font-semibold"
                style={{ width: "6.4vw", height: "4.86vh", fontSize: "1.25vw" }}
              >
                LOW VIS
              </button>

              <button
                onClick={() => { onSelect("CLOSED"); onClose(); }}
                className="flex items-center justify-center bg-[#F43A3A] text-white font-semibold"
                style={{ width: "6.4vw", height: "4.86vh", fontSize: "1.25vw" }}
              >
                CLOSED
              </button>
            </div>
          </div>

          {/* OK button */}
          <div
            className="flex justify-center"
            style={{ marginTop: "1.32vh", paddingBottom: "2.08vh" }}
          >
            <button
              onClick={onClose}
              className="flex items-center justify-center bg-[#3F3F3F] text-white font-semibold"
              style={{ width: "4.88vw", height: "4.86vh", fontSize: "1.25vw" }}
            >
              OK
            </button>
          </div>

        </div>
      </DialogContent>
    </Dialog>
  );
}
