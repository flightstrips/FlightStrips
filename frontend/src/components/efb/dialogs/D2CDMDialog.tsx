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
        className={`absolute ${position === 'L3' ? 'left-[2.5%]' : 'left-[34.58%]'} box-border flex w-[30.83%] cursor-default flex-col rounded-none border-0 bg-[#000109]`}
        onClick={(e) => e.stopPropagation()}
      >
        {/* CURRENT TOBT */}
        <div className="box-border flex h-[20%] w-full items-center justify-start gap-[5%] border-[5px] border-[#000109] bg-[#000109] px-[2%]">
          <div className="flex-1 text-center text-[clamp(5px,3.5vw,24px)] leading-[1.2] font-normal text-white">
            UPDATE TOBT
          </div>
          <div className="shrink-0 text-center text-[clamp(5px,3.5vw,18px)] font-bold text-white">
            {currentTobt}
          </div>
        </div>

        {/* CTOT */}
        <div className="box-border flex h-[15%] w-full items-center justify-start gap-[5%] border-[5px] border-[#FF9800] bg-[#FF9800] px-[2%]">
          <div className="flex-1 text-center text-[clamp(5px,3.5vw,18px)] font-normal text-[#000109]">
            CTOT
          </div>
          <div className="shrink-0 text-center text-[clamp(5px,3.5vw,18px)] font-bold text-black">
            {currentCtot}
          </div>
        </div>

        {/* EXPECTED TOBT */}
        <div className="box-border flex h-[25%] w-full items-center justify-between gap-[2%] border-[5px] border-[#011328] bg-[#011328] px-[2%]">
          {/* Time input field */}
          <input
            type="text"
            aria-label="New TOBT (UTC)"
            disabled={isSubmitting}
            value={expectedTobt}
            onChange={handleExpectedTobtChange}
            className="box-border h-full w-3/4 border-0 bg-[#E8E8E8] p-0 text-center font-mono text-[clamp(24px,5vh,80px)] font-bold text-black"
          />

          {/* Arrow buttons container */}
          <div className="flex h-[105%] w-1/4 flex-col justify-center gap-0">
            {/* Up arrow button */}
            <button
              aria-label="Increase TOBT by 5 minutes"
              disabled={isSubmitting}
              onClick={handleAdjustUp}
              className="flex h-1/2 w-full cursor-pointer items-center justify-center bg-[#1A475F] p-0 text-[clamp(5px,2.5vw,24px)] font-bold text-white"
            >
              ↑
            </button>

            {/* Down arrow button */}
            <button
              aria-label="Decrease TOBT by 5 minutes"
              disabled={isSubmitting}
              onClick={handleAdjustDown}
              className="flex h-1/2 w-full cursor-pointer items-center justify-center bg-[#1A475F] p-0 text-[clamp(5px,2.5vw,24px)] font-bold text-white"
            >
              ↓
            </button>
          </div>
        </div>

        {/* SET TOBT Buttons */}
        <div className="box-border flex h-[15%] w-full justify-between gap-0.5 border-[5px] border-[#011328] bg-[#011328] p-0.5">
          {/* SET TOBT TO NOW + 5 MIN */}
          <button
            disabled={isSubmitting}
            onClick={handleSetTobtNow5}
            className="h-full w-[calc(50%_-_1px)] cursor-pointer border-[3px] border-[#011328] bg-[#1A475F] text-center text-[clamp(5px,1vw,24px)] font-bold text-white"
          >
            SET TOBT TO NOW + 5 MIN
          </button>

          {/* SET TOBT TO NOW + 15 MIN */}
          <button
            disabled={isSubmitting}
            onClick={handleSetTobtNow15}
            className="h-full w-[calc(50%_-_1px)] cursor-pointer border-[3px] border-[#011328] bg-[#1A475F] text-center text-[clamp(5px,1vw,24px)] font-bold text-white"
          >
            SET TOBT TO NOW + 15 MIN
          </button>
        </div>

        {updateError && <div role="alert" className="absolute bottom-[20%] left-0 z-10 w-full bg-[#B63F3F] px-2 py-1 text-center font-bold text-white">{updateError}</div>}
        {!onUpdate && <div className="text-center text-sm text-white">Preview only — no update will be sent to vIFF</div>}

        {/* UPDATE Button */}
        <button
          onClick={() => void handleUpdate()}
          disabled={isSubmitting || !onUpdate}
          className="box-border h-[20%] w-full cursor-pointer border-[5px] border-[#011328] bg-[#3E5F2E] p-0 text-2xl font-bold text-white disabled:cursor-not-allowed disabled:opacity-60"
        >
          {isSubmitting ? 'UPDATING TOBT' : 'UPDATE TOBT'}
        </button>

        {/* CANCEL Button */}
        <button
          disabled={isSubmitting}
          onClick={onClose}
          className="box-border h-[20%] w-full cursor-pointer border-[5px] border-[#011328] bg-[#B63F3F] p-0 text-2xl font-bold text-white"
        >
          CANCEL
        </button>
      </div>
    </div>
  );
}
