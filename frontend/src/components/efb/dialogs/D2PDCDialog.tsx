import { useState, type ReactNode } from 'react';

type D2Position = 'L2' | 'M2' | 'R2' | 'L3' | 'M3' | 'R3';
interface Props { isOpen: boolean; onClose: () => void; onConfirm: () => Promise<void>; onUnable: () => Promise<void>; position: D2Position; content?: ReactNode; pdcText?: string; acknowledged?: boolean }
const positions: Record<D2Position, string> = {
  L2: 'left-[2.5%] top-[15%]', M2: 'left-[34.58%] top-[15%]', R2: 'left-[66.66%] top-[15%]',
  L3: 'left-[2.5%] top-[57.5%]', M3: 'left-[34.58%] top-[57.5%]', R3: 'left-[66.66%] top-[57.5%]',
};

export default function D2PDCDialog({ isOpen, onClose, onConfirm, onUnable, position, content, pdcText, acknowledged = false }: Props) {
  const [pendingAction, setPendingAction] = useState<'confirm' | 'unable' | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const runAction = async (action: 'confirm' | 'unable') => {
    if (pendingAction) return;
    setPendingAction(action);
    setActionError(null);
    try {
      await (action === 'confirm' ? onConfirm() : onUnable());
      onClose();
    } catch (error) {
      setActionError(error instanceof Error ? error.message : 'PDC action failed');
    } finally {
      setPendingAction(null);
    }
  };
  if (!isOpen) return null;
  return <div className="fixed inset-0 z-[999] flex bg-transparent" onClick={onClose}>
    <div role="dialog" aria-label="PDC clearance" className={`absolute flex h-[40%] w-[30.83%] flex-col bg-[#000109] p-1 text-white shadow-[0_10px_40px_rgba(0,0,0,0.3)] ${positions[position]}`} onClick={(event) => event.stopPropagation()}>
      <div className="sr-only">{content}</div>
      <div className="efb-scroll flex min-h-0 flex-1 overflow-auto rounded-sm border-2 border-[#484b4c] bg-[#000109] p-1">
        <div className="m-auto whitespace-pre-line text-center text-[clamp(12px,2vh,24px)] leading-[1.4] text-white">{pdcText || 'PDC CLEARANCE UNAVAILABLE'}</div>
      </div>
      {actionError && !acknowledged && <div role="alert" className="bg-[#B63F3F] px-2 py-1 text-center font-bold">{actionError}</div>}
      <div className="flex h-[18%] shrink-0">
        {acknowledged ? (
          <>
            <div className="flex w-[65%] items-center justify-center border-[5px] border-[#000109] bg-[#41826e] text-[clamp(12px,2.5vh,24px)] font-bold">ACKNOWLEDGED</div>
            <button type="button" className="w-[35%] border-[5px] border-[#000109] bg-[#1A475F] text-[clamp(12px,2.5vh,24px)] font-bold" onClick={onClose}>CLOSE</button>
          </>
        ) : (
          <>
            <button type="button" disabled={pendingAction !== null} className="w-[65%] border-[5px] border-[#000109] bg-[#41826e] text-[clamp(12px,2.5vh,24px)] font-bold disabled:cursor-not-allowed disabled:opacity-60" onClick={() => void runAction('confirm')}>{pendingAction === 'confirm' ? 'CONFIRMING' : 'CONFIRM'}</button>
            <button type="button" disabled={pendingAction !== null} className="w-[35%] border-[5px] border-[#000109] bg-[#B63F3F] text-[clamp(12px,2.5vh,24px)] font-bold disabled:cursor-not-allowed disabled:opacity-60" onClick={() => void runAction('unable')}>{pendingAction === 'unable' ? 'SENDING' : 'UNABLE'}</button>
          </>
        )}
      </div>
    </div>
  </div>;
}
