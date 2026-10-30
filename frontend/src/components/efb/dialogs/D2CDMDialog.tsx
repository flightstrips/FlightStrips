import { useState } from 'react';

interface D2CDMDialogProps {
  isOpen: boolean;
  onClose: () => void;
  position?: 'L3' | 'M2' | 'M3';
  currentTobt: string;
  currentCtot: string;
  onUpdate?: (newTobt: string) => Promise<void>;
}

function currentUtcTime(): string {
  const now = new Date();
  return `${String(now.getUTCHours()).padStart(2, '0')}${String(now.getUTCMinutes()).padStart(2, '0')}Z`;
}

export default function D2CDMDialog(props: D2CDMDialogProps) {
  return props.isOpen ? <TOBTMenu {...props} /> : null;
}

function TOBTMenu({
  onClose,
  position = 'M3',
  currentTobt,
  currentCtot,
  onUpdate,
}: D2CDMDialogProps) {
  const [expectedTobt, setExpectedTobt] = useState(currentUtcTime);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [updateError, setUpdateError] = useState<string | null>(null);

  const parseTimeToMinutes = (timeStr: string): number => {
    const match = timeStr.match(/(\d{2})(\d{2})/);
    if (!match) return 0;
    const hours = parseInt(match[1]);
    const minutes = parseInt(match[2]);
    return hours * 60 + minutes;
  };

  const minutesToTimeString = (minutes: number): string => {
    const normalized = ((minutes % 1440) + 1440) % 1440;
    const normalizedHours = Math.floor(normalized / 60);
    const normalizedMinutes = normalized % 60;
    return `${String(normalizedHours).padStart(2, '0')}${String(normalizedMinutes).padStart(2, '0')}Z`;
  };

  const getCurrentTimeInMinutes = (): number => {
    const now = new Date();
    return now.getUTCHours() * 60 + now.getUTCMinutes();
  };

  const handleSetTobtNow5 = () => {
    const currentMinutes = getCurrentTimeInMinutes();
    const newTime = minutesToTimeString(currentMinutes + 5);
    setExpectedTobt(newTime);
  };

  const handleSetTobtNow15 = () => {
    const currentMinutes = getCurrentTimeInMinutes();
    const newTime = minutesToTimeString(currentMinutes + 15);
    setExpectedTobt(newTime);
  };

  const handleAdjustUp = () => {
    const minutes = parseTimeToMinutes(expectedTobt);
    const newTime = minutesToTimeString(minutes + 5);
    setExpectedTobt(newTime);
  };

  const handleAdjustDown = () => {
    const minutes = parseTimeToMinutes(expectedTobt);
    const newTime = minutesToTimeString(minutes - 5);
    setExpectedTobt(newTime);
  };

  const handleExpectedTobtChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value.toUpperCase();
    // Allow only valid time format (HHMMZ)
    if (/^\d{0,4}Z?$/.test(value)) {
      setExpectedTobt(value);
    }
  };

  const handleUpdate = async () => {
    if (!onUpdate || isSubmitting) return;
    if (!/^([01]\d|2[0-3])[0-5]\dZ?$/.test(expectedTobt)) {
      setUpdateError('TOBT must be a valid UTC time in HHMMZ format');
      return;
    }
    setIsSubmitting(true);
    setUpdateError(null);
    try {
      await onUpdate(expectedTobt);
      onClose();
    } catch (error) {
      setUpdateError(error instanceof Error ? error.message : 'TOBT update failed');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      className="fixed top-0 left-0 z-[100] flex h-screen w-screen items-center justify-center bg-black/70"
      onClick={() => { if (!isSubmitting) onClose(); }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="TOBT"
        style={{ top: `calc(${position === 'M2' ? '15%' : '57.5%'} + clamp(32px, 4.2vh, 44px))`, height: 'calc(40% - clamp(32px, 4.2vh, 44px))' }}
        className={`absolute ${position === 'L3' ? 'left-[2.5%]' : 'left-[34.58%]'} box-border flex w-[30.83%] cursor-default flex-col gap-[clamp(3px,0.5vh,6px)] overflow-y-auto rounded-none border-0 bg-[#000109] p-[clamp(8px,1vh,12px)] text-white`}
        onClick={(e) => e.stopPropagation()}
      >
        <h2 className="shrink-0 text-[clamp(16px,2.2vh,26px)] font-bold tracking-wide">UPDATE TOBT</h2>
        <div className="grid shrink-0 grid-cols-2 gap-3 border-y border-[#43C6E7]/20 py-[clamp(5px,0.8vh,10px)]">
          <div>
            <div className="text-[clamp(10px,1.3vh,16px)] text-[#AFC9DE]">CURRENT TOBT</div>
            <div className="font-mono text-[clamp(16px,2.4vh,28px)] font-bold tabular-nums">{currentTobt}</div>
          </div>
          <div className="text-right">
            <div className="text-[clamp(10px,1.3vh,16px)] text-[#AFC9DE]">CTOT</div>
            <div className={`font-mono text-[clamp(16px,2.4vh,28px)] font-bold tabular-nums ${currentCtot === 'NIL' ? 'text-[#AFC9DE]' : 'text-[#FF9800]'}`}>{currentCtot}</div>
          </div>
        </div>
        <label htmlFor="efb-new-tobt" className="shrink-0 text-[clamp(10px,1.3vh,16px)] font-bold text-[#AFC9DE]">NEW TOBT (UTC)</label>
        <div className="flex shrink-0 items-stretch gap-2">
          <input
            id="efb-new-tobt"
            type="text"
            inputMode="numeric"
            autoComplete="off"
            spellCheck={false}
            aria-label="New TOBT (UTC)"
            disabled={isSubmitting}
            value={expectedTobt}
            onChange={handleExpectedTobtChange}
            className="min-w-0 flex-1 rounded-none border border-[#43C6E7]/50 bg-[#000109] py-1 text-center font-mono text-[clamp(22px,3.5vh,42px)] leading-none font-bold text-white tabular-nums outline-none focus:border-[#43C6E7] focus:ring-2 focus:ring-[#43C6E7]/30 disabled:opacity-60"
          />

          <div className="flex w-1/4 flex-col gap-1">
            <button
              aria-label="Increase TOBT by 5 minutes"
              disabled={isSubmitting}
              onClick={handleAdjustUp}
              className="flex flex-1 cursor-pointer items-center justify-center rounded bg-[#1A475F] text-[clamp(12px,1.8vh,22px)] font-bold hover:bg-[#2E435F] focus-visible:outline-2 focus-visible:outline-[#43C6E7] disabled:opacity-60"
            >
              +5
            </button>

            <button
              aria-label="Decrease TOBT by 5 minutes"
              disabled={isSubmitting}
              onClick={handleAdjustDown}
              className="flex flex-1 cursor-pointer items-center justify-center rounded bg-[#1A475F] text-[clamp(12px,1.8vh,22px)] font-bold hover:bg-[#2E435F] focus-visible:outline-2 focus-visible:outline-[#43C6E7] disabled:opacity-60"
            >
              -5
            </button>
          </div>
        </div>

        <div className="grid shrink-0 grid-cols-2 gap-2">
          <button
            disabled={isSubmitting}
            onClick={handleSetTobtNow5}
            aria-label="SET TOBT TO NOW + 5 MIN"
            className="cursor-pointer rounded bg-[#1A475F] py-[clamp(6px,0.8vh,12px)] text-[clamp(11px,1.6vh,20px)] font-bold hover:bg-[#2E435F] focus-visible:outline-2 focus-visible:outline-[#43C6E7] disabled:opacity-60"
          >
            NOW + 5 MIN
          </button>

          <button
            disabled={isSubmitting}
            onClick={handleSetTobtNow15}
            aria-label="SET TOBT TO NOW + 15 MIN"
            className="cursor-pointer rounded bg-[#1A475F] py-[clamp(6px,0.8vh,12px)] text-[clamp(11px,1.6vh,20px)] font-bold hover:bg-[#2E435F] focus-visible:outline-2 focus-visible:outline-[#43C6E7] disabled:opacity-60"
          >
            NOW + 15 MIN
          </button>
        </div>

        {updateError && <div role="alert" className="shrink-0 rounded bg-[#B63F3F] px-3 py-2 text-sm font-bold text-white">{updateError}</div>}
        {!onUpdate && <div className="text-center text-sm text-white">Preview only — no update will be sent to vIFF</div>}

        <div className="grid shrink-0 grid-cols-[1.5fr_1fr] gap-2 pt-1">
        <button
          onClick={() => void handleUpdate()}
          disabled={isSubmitting || !onUpdate}
          className="cursor-pointer rounded bg-[#41826E] py-[clamp(8px,1vh,14px)] text-[clamp(12px,1.8vh,22px)] font-bold hover:brightness-110 focus-visible:outline-2 focus-visible:outline-[#43C6E7] disabled:cursor-not-allowed disabled:opacity-60"
        >
          {isSubmitting ? 'UPDATING TOBT' : 'UPDATE TOBT'}
        </button>

        <button
          disabled={isSubmitting}
          onClick={onClose}
          className="cursor-pointer rounded border border-[#AFC9DE]/40 bg-transparent py-[clamp(8px,1vh,14px)] text-[clamp(12px,1.8vh,22px)] font-bold hover:bg-[#1A475F] focus-visible:outline-2 focus-visible:outline-[#43C6E7] disabled:opacity-60"
        >
          CANCEL
        </button>
        </div>
      </div>
    </div>
  );
}
