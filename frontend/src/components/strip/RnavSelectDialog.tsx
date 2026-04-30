import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { RNAV_CAPABILITIES, type RnavCapability } from "@/lib/rnav";

// Styled to match Group 73CLX Sub Boxes.svg.
const CLS_DIALOG_BG = "bg-[#B3B3B3] border border-black p-0 w-[8.7vw] max-w-none max-h-none gap-0 overflow-hidden [&>button]:hidden";
const CLS_PANEL = "mx-[0.42vw] my-[1.02dvh] border border-black flex flex-col justify-between h-fit";
const CLS_RNAV_LIST = "flex flex-col items-center gap-[1.11dvh] pt-[1.3dvh] pb-[1.48dvh]";
const CLS_RNAV_BTN = "w-[5.63vw] h-[6.48dvh] bg-[#CCCCCC] text-black font-semibold text-[1.25vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";
const CLS_RNAV_BTN_ACTIVE = "w-[5.63vw] h-[6.48dvh] bg-[#1BFF16] text-black font-semibold text-[1.25vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";
const CLS_ESC_ROW = "flex items-center justify-center pb-[2.22dvh]";
const CLS_ESC_BTN = "w-[5.63vw] h-[6.48dvh] bg-[#3F3F3F] text-white font-semibold text-[1.46vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-75";

interface RnavSelectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: string | undefined | null;
  onSelect: (capability: RnavCapability) => void;
}

function isRnavCapability(value: string | undefined | null): value is RnavCapability {
  return RNAV_CAPABILITIES.includes(value as RnavCapability);
}

function rnavLabel(capability: RnavCapability) {
  return capability === "NIL" ? "NIL" : `RNAV ${capability}`;
}

export function RnavSelectDialog({
  open,
  onOpenChange,
  value,
  onSelect,
}: RnavSelectDialogProps) {
  const currentCapability: RnavCapability = isRnavCapability(value) ? value : "NIL";

  function handleSelect(capability: RnavCapability) {
    onSelect(capability);
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG_BG}>
        <DialogTitle className="sr-only">Select RNAV capability</DialogTitle>
        <div className={CLS_PANEL}>
          <div className={CLS_RNAV_LIST}>
            {RNAV_CAPABILITIES.map((capability) => (
              <button
                key={capability}
                type="button"
                className={capability === currentCapability ? CLS_RNAV_BTN_ACTIVE : CLS_RNAV_BTN}
                onClick={() => handleSelect(capability)}
              >
                {rnavLabel(capability)}
              </button>
            ))}
          </div>
          <div className={CLS_ESC_ROW}>
            <button
              type="button"
              className={CLS_ESC_BTN}
              onClick={() => onOpenChange(false)}
            >
              ESC
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
