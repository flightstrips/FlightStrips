import { useState, type CSSProperties } from 'react';
import * as Dialog from '@radix-ui/react-dialog';
import { Check, X } from 'lucide-react';
import standArrow from '@/assets/stand-arrow.png';

interface D1StandProps {
  isOpen: boolean;
  onClose: () => void;
  stand: string;
  onRequest?: (stand: string, allowUnsuitable?: boolean) => Promise<void>;
  availability?: StandAvailability[];
  availabilityLoading?: boolean;
  availabilityUnavailable?: boolean;
}

export interface StandAvailability {
  stand: string;
  available: boolean;
  reason?: string;
  reserved_by?: string[];
  unsuitable?: boolean;
  selectable?: boolean;
}

const EST_BOARD_WIDTH = 2560;
const EST_BOARD_HEIGHT = 1440;
const RAW_STANDS = [
  { label: 'A18', x: 461.851, y: 229 },
  { label: 'A19', x: 371.48, y: 229 },
  { label: 'A20', x: 281.11, y: 229 },
  { label: 'A21', x: 190.74, y: 229 },
  { label: 'A22', x: 100.37, y: 229 },
  { label: 'A23', x: 10, y: 229 },
  { label: 'W1', x: 16, y: 1015.94 },
  { label: 'RI', x: 16, y: 862.626 },
  { label: 'RII', x: 16, y: 709.313 },
  { label: 'RIII', x: 16, y: 556 },
  { label: 'A25', x: 735.145, y: 10 },
  { label: 'A26', x: 644.774, y: 10 },
  { label: 'A7', x: 670.633, y: 389 },
  { label: 'A4', x: 580.263, y: 388.921 },
  { label: 'B4', x: 895, y: 489 },
  { label: 'C27', x: 1393, y: 167 },
  { label: 'D1', x: 1483.89, y: 14 },
  { label: 'D2', x: 1574.26, y: 14 },
  { label: 'D3', x: 1664.63, y: 14 },
  { label: 'D4', x: 1755, y: 14 },
  { label: 'E25', x: 1935.48, y: 14 },
  { label: 'E20', x: 1844, y: 167 },
  { label: 'E27', x: 1935.48, y: 167.313 },
  { label: 'E22', x: 1844, y: 320.313 },
  { label: 'E29', x: 1935.48, y: 320.626 },
  { label: 'E24', x: 1844, y: 473.626 },
  { label: 'E31', x: 1934.48, y: 473.939 },
  { label: 'E36', x: 1844, y: 626.939 },
  { label: 'E33', x: 1935.48, y: 627.252 },
  { label: 'E35', x: 1935.48, y: 780.565 },
  { label: 'H105', x: 2180, y: 320.313 },
  { label: 'E90', x: 2442, y: 10 },
  { label: 'E78', x: 2340, y: 10 },
  { label: 'E89', x: 2442, y: 163.313 },
  { label: 'E77', x: 2340, y: 163.313 },
  { label: 'E88', x: 2442, y: 316.626 },
  { label: 'E76', x: 2340, y: 316.626 },
  { label: 'E87', x: 2442, y: 469.939 },
  { label: 'E75', x: 2340, y: 469.939 },
  { label: 'E86', x: 2442, y: 623.252 },
  { label: 'E74', x: 2340, y: 623.252 },
  { label: 'E85', x: 2442, y: 776.565 },
  { label: 'E73', x: 2340, y: 776.565 },
  { label: 'E84', x: 2442, y: 929.878 },
  { label: 'E72', x: 2340, y: 929.878 },
  { label: 'E83', x: 2442, y: 1083.19 },
  { label: 'E71', x: 2340, y: 1083.19 },
  { label: 'E82', x: 2441, y: 1236.5 },
  { label: 'E70', x: 2339, y: 1236.5 },
  { label: 'H106', x: 2180, y: 167 },
  { label: 'H103', x: 2180, y: 626.939 },
  { label: 'H104', x: 2180, y: 473.626 },
  { label: 'H101', x: 2180.22, y: 933.565 },
  { label: 'F9', x: 2089.85, y: 933.565 },
  { label: 'F8', x: 1999.48, y: 933.565 },
  { label: 'F7', x: 1909.11, y: 933.565 },
  { label: 'F5', x: 1818.74, y: 933.565 },
  { label: 'F89', x: 2059.84, y: 1101.878 },
  { label: 'F91', x: 1969.47, y: 1101.878 },
  { label: 'F93', x: 1879.1, y: 1101.878 },
  { label: 'F95', x: 1788.73, y: 1101.878 },
  { label: 'F97', x: 1698.36, y: 1101.878 },
  { label: 'F90', x: 2017.48, y: 1254.878 },
  { label: 'F92', x: 1927.11, y: 1254.878 },
  { label: 'F94', x: 1836.74, y: 1254.878 },
  { label: 'F96', x: 1746.37, y: 1254.878 },
  { label: 'F98', x: 1656, y: 1254.878 },
  { label: 'F4', x: 1728.37, y: 933.565 },
  { label: 'F1', x: 1638, y: 933.565 },
  { label: 'H102', x: 2180, y: 780.252 },
  { label: 'C29', x: 1393, y: 320 },
  { label: 'C33', x: 1392, y: 473 },
  { label: 'C35', x: 1391, y: 626 },
  { label: 'C37', x: 1391, y: 779 },
  { label: 'C39', x: 1391, y: 932 },
  { label: 'C28', x: 1300, y: 350 },
  { label: 'C30', x: 1301, y: 503 },
  { label: 'C32', x: 1301, y: 656 },
  { label: 'C34', x: 1301, y: 809 },
  { label: 'C36', x: 1301.24, y: 962.1 },
  { label: 'B6', x: 895, y: 642 },
  { label: 'B8', x: 895, y: 795 },
  { label: 'B10', x: 896, y: 948 },
  { label: 'B19', x: 892, y: 1101.42 },
  { label: 'B17', x: 982.76, y: 1101.42 },
  { label: 'B15', x: 1073.13, y: 1101 },
  { label: 'B7', x: 988, y: 746.626 },
  { label: 'B9', x: 988, y: 899.939 },
  { label: 'A9', x: 648.37, y: 543 },
  { label: 'A6', x: 558, y: 543 },
  { label: 'A11', x: 635.884, y: 696 },
  { label: 'A8', x: 546, y: 696 },
  { label: 'A15', x: 473.883, y: 849 },
  { label: 'A17', x: 384, y: 849 },
  { label: 'A12', x: 654.624, y: 849 },
  { label: 'A14', x: 564.253, y: 849 },
  { label: 'A27', x: 554.404, y: 10 },
  { label: 'A28', x: 464.034, y: 10 },
  { label: 'A30', x: 373.664, y: 26 },
  { label: 'A31', x: 283.294, y: 55.823 },
  { label: 'A32', x: 192.767, y: 56 },
  { label: 'A33', x: 103, y: 56 },
  { label: 'A34', x: 13, y: 56 },
];

const CARGO_STANDS = [
  { label: 'G117', x: 185, y: 12 },
  { label: 'G118', x: 239, y: 34 },
  { label: 'G119', x: 293, y: 12 },
  { label: 'G120', x: 359, y: 12 },
  { label: 'G121', x: 412, y: 34 },
  { label: 'G122', x: 466, y: 12 },
  { label: 'G123', x: 532, y: 12 },
  { label: 'G124', x: 586, y: 34 },
  { label: 'G125', x: 639, y: 12 },
  { label: 'G126', x: 705, y: 12 },
  { label: 'G127', x: 759, y: 34 },
  { label: 'G128', x: 813, y: 12 },
  { label: 'G129', x: 879, y: 12 },
  { label: 'G130', x: 932, y: 34 },
  { label: 'G131', x: 986, y: 12 },
  { label: 'G132', x: 1052, y: 12 },
  { label: 'G133', x: 1106, y: 34 },
  { label: 'G134', x: 1159, y: 12 },
  { label: 'G135', x: 1225, y: 12 },
  { label: 'G136', x: 1279, y: 34 },
  { label: 'G137', x: 1333, y: 12 },
  { label: 'G114', x: 98, y: 212 },
  { label: 'G113', x: 98, y: 307 },
  { label: 'G112', x: 98, y: 402 },
  { label: 'G111', x: 78, y: 497 },
  { label: 'G110', x: 78, y: 591 },
  { label: 'G15', x: 1451, y: 212 },
  { label: 'G16', x: 1451, y: 307 },
  { label: 'G17', x: 1451, y: 402 },
  { label: 'G18', x: 1451, y: 497 },
  { label: 'G19', x: 1451, y: 591 },
];

export default function D1Stand({ isOpen, onClose, stand, onRequest, availability, availabilityLoading = false, availabilityUnavailable = false }: D1StandProps) {
  const [showCargo, setShowCargo] = useState(() => CARGO_STANDS.some((entry) => entry.label === stand));
  const [selectedStand, setSelectedStand] = useState(stand);
  const [previousStand, setPreviousStand] = useState(stand);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [requestError, setRequestError] = useState<string | null>(null);
  const [confirmUnsuitable, setConfirmUnsuitable] = useState(false);
  const checking = availabilityLoading || availability === undefined;
  const selected = availability?.find((entry) => entry.stand === selectedStand);
  const selectedSelectable = selected?.available === true || (selected?.unsuitable === true && selected?.selectable === true);
  const canSubmit = selectedSelectable && !checking && !availabilityUnavailable && !isSubmitting && onRequest !== undefined;
  const availabilityByStand = new Map(availability?.map((entry) => [entry.stand, entry]));
  const displayedStands = showCargo ? CARGO_STANDS : RAW_STANDS;
  const availableCount = displayedStands.filter((entry) => availabilityByStand.get(entry.label)?.available).length;

  const handleRequest = async (confirmed = false) => {
    if (!canSubmit || !onRequest) return;
    if (selected?.unsuitable && !confirmed) {
      setConfirmUnsuitable(true);
      return;
    }
    setConfirmUnsuitable(false);
    setIsSubmitting(true);
    setRequestError(null);
    try {
      if (selected?.unsuitable) await onRequest(selectedStand, true);
      else await onRequest(selectedStand);
      onClose();
    } catch (error) {
      setRequestError(error instanceof Error ? error.message : 'Stand request was rejected');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog.Root open={isOpen} onOpenChange={(open) => { if (!open && !isSubmitting) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[1000] bg-black/75 backdrop-blur-sm" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-[1001] flex h-[96dvh] w-[98vw] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-2xl border border-[#B8D9F0]/30 bg-[#011328] text-white shadow-2xl">
          <header className="flex shrink-0 items-start justify-between gap-[12.8px] border-b border-white/10 px-4 py-[12.8px] sm:px-[22.4px]">
            <div>
              <Dialog.Title className="flex items-center gap-2 text-xl font-bold tracking-wide">
                <img src={standArrow} alt="" className="h-6 w-6 shrink-0 object-contain" />
                SELECT STAND
              </Dialog.Title>
              <Dialog.Description className="sr-only">
                Choose a stand on the map. Gray stands require suitability confirmation; reserved or blocked stands cannot be selected.
              </Dialog.Description>
            </div>
            <Dialog.Close disabled={isSubmitting} aria-label="Close stand selection" className="rounded-lg border border-white/20 p-[6.4px] text-[#B8D9F0] transition-colors hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-[#9BE9FF] disabled:opacity-40">
              <X size={20} aria-hidden="true" />
            </Dialog.Close>
          </header>

          <div className="flex shrink-0 flex-wrap items-center gap-x-[19.2px] gap-y-[6.4px] border-b border-white/10 bg-white/[0.03] px-4 py-[9.6px] text-sm sm:px-[22.4px]">
            <span className="text-[#B8D9F0]">CURRENT <strong className="ml-2 text-white">{stand || 'TBA'}</strong></span>
            <span className="text-[#B8D9F0]">SELECTED <strong className="ml-2 text-[#9BE9FF]">{selectedSelectable ? selectedStand : 'NONE'}</strong></span>
            <div className="flex flex-wrap items-center gap-4 text-xs sm:ml-auto" role="status" aria-label="Stand availability summary">
              <span className="flex items-center gap-2"><span className="h-3 w-3 rounded bg-[#41826E]" />{availabilityUnavailable ? 'UNAVAILABLE' : checking ? 'CHECKING AVAILABILITY' : `${availableCount} AVAILABLE`}</span>
              <span className="flex items-center gap-2"><span className="h-3 w-3 rounded bg-[#9E4852]" />RESERVED</span>
              <span className="flex items-center gap-2"><span className="h-3 w-3 rounded bg-[#374151]" />NOT SUITABLE</span>
              <span className="flex items-center gap-2"><span className="h-3 w-3 rounded bg-[#172333]" />BLOCKED</span>
            </div>
          </div>

          <div className="flex min-h-0 flex-1 flex-col gap-[9.6px] px-4 py-[12.8px] sm:px-[22.4px]" aria-busy={checking && !availabilityUnavailable}>
            {availabilityUnavailable ? (
              <p role="alert" className="rounded-lg border border-[#D9C01E]/40 bg-[#D9C01E]/10 p-4 text-[#F4DF79]">Stand availability is currently unavailable. Close this window and try again.</p>
            ) : checking ? (
              <p className="py-12 text-center text-[#B8D9F0]">Checking stand availability...</p>
            ) : null}
            <div role="group" aria-label={showCargo ? 'Cargo stand map' : 'Airport stand map'} className="relative mx-auto min-h-0 w-full flex-1 border border-[#B8D9F0]/25 bg-[#7b7b7b]">
                    {displayedStands.map((standDef) => {
                      const entry = availabilityByStand.get(standDef.label);
                      const isSelected = selectedStand === standDef.label;
                      const available = !checking && !availabilityUnavailable && entry?.available === true;
                      const reserved = !checking && !availabilityUnavailable && !available && !!entry?.reserved_by?.length;
                      const unsuitable = !checking && !availabilityUnavailable && entry?.unsuitable === true && entry?.selectable === true && !reserved;
                      const disabled = (!available && !unsuitable) || isSubmitting;
                      const title = availabilityUnavailable ? 'Availability is currently unavailable' : checking ? 'Checking availability...' : available ? 'Available for this flight' : `${unsuitable ? 'Not suitable' : 'Unavailable'}: ${entry?.reason || 'not configured for this flight'}`;
                      const color = available ? 'border-[#70BBA1] bg-[#41826E] hover:bg-[#4B947D]' : reserved ? 'border-[#BE6A73] bg-[#622E3A]' : unsuitable ? 'border-[#556274] bg-[#374151] text-[#B8C5D5] hover:bg-[#465368]' : 'border-[#556274] bg-[#172333] text-[#B8C5D5]';
                      const position = {
                        '--stand-left': `${(standDef.x / (showCargo ? 1566 : EST_BOARD_WIDTH)) * 100}%`,
                        '--stand-top': `${(standDef.y / (showCargo ? 707 : EST_BOARD_HEIGHT)) * 100}%`,
                      } as CSSProperties;
                      return (
                        <button
                          key={standDef.label}
                          type="button"
                          aria-label={`Stand ${standDef.label}`}
                          aria-pressed={isSelected}
                          disabled={disabled}
                          title={`${standDef.label}: ${title}`}
                          style={position}
                          onClick={() => {
                            if (!selected?.unsuitable) setPreviousStand(selectedStand);
                            setSelectedStand(standDef.label);
                            setRequestError(null);
                            setConfirmUnsuitable(false);
                          }}
                          className={`absolute [left:var(--stand-left)] [top:var(--stand-top)] flex ${showCargo ? 'h-[12.45%] w-[3.2%]' : 'h-[10%] w-[3%] translate-[10%]'} flex-col items-center justify-between overflow-hidden rounded-md border px-0.5 py-1 text-center [container-type:inline-size] transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#9BE9FF] ${color} ${disabled ? 'cursor-not-allowed' : 'cursor-pointer'} ${isSelected ? 'z-10 ring-2 ring-[#9BE9FF] ring-offset-1 ring-offset-[#011328]' : ''}`}
                        >
                          <span className="text-[clamp(10px,0.85vw,15px)] font-bold leading-tight">{standDef.label}</span>
                          {reserved && <span className="w-full">
                            {entry?.reserved_by?.map((callsign) => (
                              <span key={callsign} className="block whitespace-nowrap font-mono font-semibold leading-tight" style={{ fontSize: `min(11px, ${100 / (Math.max(1, callsign.length) * 0.65)}cqw)` }}>{callsign}</span>
                            ))}
                          </span>}
                          {isSelected && <span className="flex h-[45%] w-full items-center justify-center rounded-sm bg-[#70BBA1]/40">
                            <Check size={14} role="img" aria-label="Selected stand" />
                          </span>}
                        </button>
                      );
                    })}
            </div>
          </div>

          <footer className="shrink-0 border-t border-white/10 bg-[#071D33] px-4 py-[12.8px] sm:px-[22.4px]">
            {requestError && <div role="alert" className="mb-3 rounded-lg border border-red-400/40 bg-[#622E3A] px-4 py-3 text-sm font-semibold">{requestError}</div>}
            <div className="flex flex-wrap items-center justify-between gap-[9.6px]">
              <button type="button" disabled={isSubmitting} onClick={() => setShowCargo((current) => !current)} className="min-w-32 rounded-lg border border-[#B8D9F0]/35 px-4 py-[9.6px] text-sm font-bold hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-[#9BE9FF] disabled:opacity-40">
                {showCargo ? 'APRON' : 'CARGO'}
              </button>
              <div className="flex gap-[9.6px]">
                <Dialog.Close disabled={isSubmitting} className="rounded-lg border border-[#B8D9F0]/35 px-4 py-[9.6px] text-sm font-bold hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-[#9BE9FF] disabled:opacity-40">CANCEL</Dialog.Close>
                <button type="button" onClick={() => void handleRequest()} disabled={!canSubmit} className="rounded-lg border border-[#70BBA1]/60 bg-[#41826E] px-4 py-[9.6px] text-sm font-bold transition-colors hover:bg-[#4B947D] focus-visible:outline-2 focus-visible:outline-[#9BE9FF] disabled:cursor-not-allowed disabled:border-white/10 disabled:bg-[#293A4C] disabled:text-[#93A5B9]">
                  {isSubmitting ? 'CHECKING STAND' : checking && !availabilityUnavailable ? 'LOADING AVAILABILITY' : 'REQUEST NEW STAND'}
                </button>
              </div>
            </div>
          </footer>
          <Dialog.Root open={confirmUnsuitable} onOpenChange={(open) => {
            setConfirmUnsuitable(open);
            if (!open) {
              setSelectedStand(previousStand);
              setShowCargo(CARGO_STANDS.some((entry) => entry.label === previousStand));
            }
          }}>
            <Dialog.Portal>
              <Dialog.Overlay className="fixed inset-0 z-[1002] bg-black/70" />
              <Dialog.Content className="fixed top-1/2 left-1/2 z-[1003] w-[min(92vw,480px)] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-[#D9C01E]/60 bg-[#071D33] p-6 text-white shadow-2xl">
                <Dialog.Title className="text-lg font-bold text-[#F4DF79]">Stand not suitable</Dialog.Title>
                <Dialog.Description className="mt-3 text-[#B8D9F0]">This stand is not suitable for your flight. Select anyway?</Dialog.Description>
                <p className="mt-2 text-sm text-[#B8D9F0]">{selectedStand}: {selected?.reason}</p>
                <div className="mt-6 flex justify-end gap-3">
                  <Dialog.Close autoFocus className="rounded-lg border border-[#B8D9F0]/35 px-4 py-3 text-sm font-bold hover:bg-white/10">CANCEL</Dialog.Close>
                  <button type="button" onClick={() => void handleRequest(true)} className="rounded-lg border border-[#D9C01E]/60 bg-[#D9C01E]/20 px-4 py-3 text-sm font-bold text-[#F4DF79] hover:bg-[#D9C01E]/30">SELECT ANYWAY</button>
                </div>
              </Dialog.Content>
            </Dialog.Portal>
          </Dialog.Root>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
