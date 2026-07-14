import type { ReactNode } from 'react';

type D2Position = 'L2' | 'M2' | 'R2' | 'L3' | 'M3' | 'R3';

interface D2ATISDialogProps {
  isOpen: boolean;
  onClose: () => void;
  position: D2Position;
  content?: ReactNode;
  atisText?: string;
}

const positions: Record<D2Position, string> = {
  L2: 'left-[2.5%] top-[15%]', M2: 'left-[34.58%] top-[15%]', R2: 'left-[66.66%] top-[15%]',
  L3: 'left-[2.5%] top-[57.5%]', M3: 'left-[34.58%] top-[57.5%]', R3: 'left-[66.66%] top-[57.5%]',
};

export default function D2ATISDialog({ isOpen, onClose, position, content, atisText }: D2ATISDialogProps) {
  if (!isOpen) return null;
  return <div className="fixed inset-0 z-[999] flex bg-transparent" onClick={onClose}>
    <div className={`absolute h-[40%] w-[30.83%] overflow-auto bg-[#000109] p-5 text-white shadow-[0_10px_40px_rgba(0,0,0,0.3)] ${positions[position]}`} onClick={(event) => event.stopPropagation()}>
      <div className="mt-5 text-[clamp(12px,2vh,16px)] leading-[1.5] text-[#e0e0e0]">{content}</div>
      <div className="absolute left-[5%] top-[5%] flex h-[70%] w-[90%] items-center justify-center overflow-auto rounded-sm border-2 border-[#484b4c] bg-[#000109] p-0.5">
        <div className="whitespace-pre-line text-center text-[clamp(10px,1.2vh,32px)] leading-[1.6] text-white">{atisText || 'ATIS UNAVAILABLE'}</div>
      </div>
      <button className="absolute left-[5%] top-[80%] h-[15%] w-[90%] border-[5px] border-[#000109] bg-[#41826e] p-0 text-2xl font-bold text-white" onClick={onClose}>CLOSE</button>
    </div>
  </div>;
}
