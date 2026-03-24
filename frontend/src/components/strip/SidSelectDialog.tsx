import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { MOCK_SIDS } from "./sidConstants";

// Tailwind class constants (hex must be literal strings for JIT) — styled to match SIDS.svg
const CLS_DIALOG_BG      = "bg-[#B3B3B3] border border-black p-0 w-[288px] max-w-none max-h-none gap-0 overflow-hidden [&>button]:hidden";
const CLS_PANEL          = "mx-[12px] mt-[18px] mb-0 border border-black flex flex-col justify-between h-fit";
const CLS_SID_LIST       = "flex-1 flex flex-col items-center gap-[12px] pt-[20px] pb-[12px] overflow-y-auto";
const CLS_SID_BTN        = "w-[201px] h-[48px] bg-[#D6D6D6] text-black font-semibold text-[24px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-95";
const CLS_SID_BTN_ACTIVE = "w-[201px] h-[48px] bg-[#1BFF16] text-black font-semibold text-[24px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-95";
const CLS_BOTTOM_ROW     = "flex items-center justify-around px-[24px] pb-[24px] pt-[8px]";
const CLS_BOTTOM_BTN     = "w-[95px] h-[53px] bg-[#3F3F3F] text-white font-semibold text-[24px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";

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
