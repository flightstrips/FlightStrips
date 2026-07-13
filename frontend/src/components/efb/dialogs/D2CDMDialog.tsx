import { useState, useEffect } from 'react';

interface D2CDMDialogProps {
  isOpen: boolean;
  onClose: () => void;
  currentTobt: string;
  currentCtot: string;
  onUpdate?: (newTobt: string) => Promise<void>;
}

export default function D2CDMDialog({
  isOpen,
  onClose,
  currentTobt,
  currentCtot,
  onUpdate,
}: D2CDMDialogProps) {
  const [expectedTobt, setExpectedTobt] = useState(currentTobt);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [updateError, setUpdateError] = useState<string | null>(null);

  useEffect(() => {
    if (isOpen) {
      const timer = window.setTimeout(() => setExpectedTobt(currentTobt), 0);
      return () => window.clearTimeout(timer);
    }
  }, [isOpen, currentTobt]);

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
    if (!/^([01]\d|2[0-3])[0-5]\dZ?$/.test(expectedTobt)) {
      setUpdateError('TOBT must be a valid UTC time in HHMMZ format');
      return;
    }
    setIsSubmitting(true);
    setUpdateError(null);
    try {
      await onUpdate?.(expectedTobt);
      onClose();
    } catch (error) {
      setUpdateError(error instanceof Error ? error.message : 'TOBT update failed');
    } finally {
      setIsSubmitting(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div
      className="fixed top-0 left-0 z-[100] flex h-screen w-screen items-center justify-center bg-black/70"
      onClick={onClose}
    >
      <div
        className="absolute top-[57.5%] left-[34.58%] flex h-[40%] w-[30.83%] cursor-default flex-col border-2 border-[#000109] bg-[#000109]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* CURRENT TOBT */}
        <div className="box-border flex h-[20%] w-full items-center justify-start gap-[5%] border-[5px] border-[#000109] bg-[#000109] px-[2%]">
          <div className="w-full text-center text-[clamp(5px,3.5vw,24px)] leading-[1.2] font-normal text-white">
            UPDATE TOBT
          </div>
          <div className="flex-1 text-center text-[clamp(5px,3.5vw,18px)] font-bold text-white">
            {currentTobt}
          </div>
        </div>

        {/* CTOT */}
        <div className="box-border flex h-[15%] w-full items-center justify-start gap-[5%] border-[5px] border-[#FF9800] bg-[#FF9800] px-[2%]">
          <div className="w-full text-center text-[clamp(5px,3.5vw,18px)] font-normal text-[#000109]">
            CTOT
          </div>
          <div className="flex-1 text-center text-[clamp(5px,3.5vw,18px)] font-bold text-black">
            {currentCtot}
          </div>
        </div>

        {/* EXPECTED TOBT */}
        <div className="box-border flex h-[25%] w-full items-center justify-between gap-[2%] border-[5px] border-[#011328] bg-[#011328] px-[2%]">
          {/* Time input field */}
          <input
            type="text"
            value={expectedTobt}
            onChange={handleExpectedTobtChange}
            className="box-border h-full w-3/4 border-0 bg-[#E8E8E8] p-0 text-center font-mono text-[80px] font-bold text-black"
          />

          {/* Arrow buttons container */}
          <div className="flex h-[105%] w-1/4 flex-col justify-center gap-0">
            {/* Up arrow button */}
            <button
              onClick={handleAdjustUp}
              className="flex h-1/2 w-full cursor-pointer items-center justify-center bg-[#1A475F] p-0 text-[clamp(5px,2.5vw,24px)] font-bold text-white"
            >
              ↑
            </button>

            {/* Down arrow button */}
            <button
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
            onClick={handleSetTobtNow5}
            className="h-full w-[calc(50%_-_1px)] cursor-pointer border-[3px] border-[#011328] bg-[#1A475F] text-center text-[clamp(5px,1vw,24px)] font-bold text-white"
          >
            SET TOBT TO NOW + 5 MIN
          </button>

          {/* SET TOBT TO NOW + 15 MIN */}
          <button
            onClick={handleSetTobtNow15}
            className="h-full w-[calc(50%_-_1px)] cursor-pointer border-[3px] border-[#011328] bg-[#1A475F] text-center text-[clamp(5px,1vw,24px)] font-bold text-white"
          >
            SET TOBT TO NOW + 15 MIN
          </button>
        </div>

        {updateError && <div role="alert" className="absolute bottom-[20%] left-0 z-10 w-full bg-[#B63F3F] px-2 py-1 text-center font-bold text-white">{updateError}</div>}

        {/* UPDATE Button */}
        <button
          onClick={() => void handleUpdate()}
          disabled={isSubmitting}
          className="box-border h-[20%] w-full cursor-pointer border-[5px] border-[#011328] bg-[#3E5F2E] p-0 text-2xl font-bold text-white disabled:cursor-not-allowed disabled:opacity-60"
        >
          {isSubmitting ? 'UPDATING TOBT' : 'UPDATE TOBT'}
        </button>

        {/* CANCEL Button */}
        <button
          onClick={onClose}
          className="box-border h-[20%] w-full cursor-pointer border-[5px] border-[#011328] bg-[#B63F3F] p-0 text-2xl font-bold text-white"
        >
          CANCEL
        </button>
      </div>
    </div>
  );
}
