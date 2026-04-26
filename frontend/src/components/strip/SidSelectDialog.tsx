import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { MOCK_SIDS } from "./sidConstants";

// All sizes derived from 1920×1080 base:
//   horizontal → / 1920 * 100 = vw
//   vertical   → / 1080 * 100 = dvh
//
// Dialog: 288px → 15vw
// Panel mx: 12px → 0.63vw  |  mt: 18px → 1.67dvh
// List gap: 12px → 1.11dvh  |  pt: 20px → 1.85dvh  |  pb: 12px → 1.11dvh
// SID button: 201×48px → 10.47vw × 4.44dvh  |  font: 24px → 1.25vw
// Bottom row: px: 24px → 1.25vw  |  pb: 24px → 2.22dvh  |  pt: 8px → 0.74dvh
// Bottom button: 95×53px → 4.95vw × 4.91dvh  |  font: 24px → 1.25vw

// Tailwind class constants (hex must be literal strings for JIT) — styled to match SIDS.svg
const CLS_DIALOG_BG      = "bg-[#B3B3B3] border border-black p-0 w-[15vw] max-w-none max-h-none gap-0 overflow-hidden [&>button]:hidden";
const CLS_PANEL          = "mx-[0.63vw] mt-[1.67dvh] mb-0 border border-black flex flex-col justify-between h-fit";
const CLS_SID_LIST       = "flex-1 flex flex-col items-center gap-[1.11dvh] pt-[1.85dvh] pb-[1.11dvh] overflow-y-auto";
const CLS_SID_BTN        = "w-[10.47vw] h-[4.44dvh] bg-[#D6D6D6] text-black font-semibold text-[1.25vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-95";
const CLS_SID_BTN_ACTIVE = "w-[10.47vw] h-[4.44dvh] bg-[#1BFF16] text-black font-semibold text-[1.25vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-95";
const CLS_BOTTOM_ROW     = "flex items-center justify-around px-[1.25vw] pb-[2.22dvh] pt-[0.74dvh]";
const CLS_BOTTOM_BTN     = "w-[4.95vw] h-[4.91dvh] bg-[#3F3F3F] text-white font-semibold text-[1.25vw] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";

interface SidSelectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: string | undefined | null;
  onSelect: (sid: string) => void;
  /** Optional handler for ERASE button; falls back to just closing if omitted. */
  onErase?: () => void;
  /** SIDs to show; defaults to MOCK_SIDS. Replace with API data when backend is ready. */
  sids?: string[];
}

export function SidSelectDialog({
  open,
  onOpenChange,
  value,
  onSelect,
  onErase,
  sids = MOCK_SIDS,
}: SidSelectDialogProps) {
  const currentSid = value ?? undefined;

  function handleSelect(sid: string) {
    onSelect(sid);
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG_BG}>
        <DialogTitle className="sr-only">Select SID</DialogTitle>
        <div className={CLS_PANEL}>
          <div className={CLS_SID_LIST}>
            {sids.map((sid) => (
              <button
                key={sid}
                type="button"
                className={sid === currentSid ? CLS_SID_BTN_ACTIVE : CLS_SID_BTN}
                onClick={() => handleSelect(sid)}
              >
                {sid}
              </button>
            ))}
          </div>
          <div className={CLS_BOTTOM_ROW}>
            <button
              type="button"
              className={CLS_BOTTOM_BTN}
              onClick={() => {
                onErase?.();
                if (!onErase) onOpenChange(false);
              }}
            >
              ERASE
            </button>
            <button
              type="button"
              className={CLS_BOTTOM_BTN}
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
