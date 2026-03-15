import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";

/** Mock SIDs for the active departure runway. Replace with backend data when available. */
export const MOCK_SIDS = [
  "LANGO2C",
  "GOLGA2C",
  "KEMAX2C",
  "SIMEG9C",
  "SALLO1C",
  "ODDON2C",
];

// Tailwind class constants (hex must be literal strings for JIT) — same style as RunwayDialog
const CLS_DIALOG_BG      = "bg-[#B3B3B3] border border-black p-0 w-[200px] gap-0 overflow-hidden [&>button]:hidden";
const CLS_SID_BTN        = "w-full h-[70px] bg-[#CCCCCC] text-black font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";
const CLS_SID_BTN_ACTIVE = "w-full h-[70px] bg-[#2CBB00] text-white font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-90";
const CLS_ESC_BTN        = "w-full h-[70px] bg-[#3F3F3F] text-white font-semibold text-[28px] shadow-[0_4px_4px_rgba(0,0,0,0.25)] outline-none active:brightness-75";

interface SidSelectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  value: string | undefined | null;
  onSelect: (sid: string) => void;
  /** SIDs to show; defaults to MOCK_SIDS. Replace with API data when backend is ready. */
  sids?: string[];
}

export function SidSelectDialog({
  open,
  onOpenChange,
  value,
  onSelect,
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
        <div className="border border-black mx-[7px] mt-[11px] mb-0 flex flex-col gap-0 p-[9px] pb-0">
          {sids.map((sid) => (
            <div key={sid} className="pb-[9px]">
              <button
                type="button"
                className={sid === currentSid ? CLS_SID_BTN_ACTIVE : CLS_SID_BTN}
                onClick={() => handleSelect(sid)}
              >
                {sid}
              </button>
            </div>
          ))}
          <div className="pb-[9px]">
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
