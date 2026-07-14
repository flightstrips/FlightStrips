import { useState, type CSSProperties } from 'react';

interface RawStandDefinition {
  label: string;
  x: number;
  y: number;
}

interface D1StandProps {
  isOpen: boolean;
  onClose: () => void;
  stand: string;
  onRequest?: (stand: string) => Promise<void>;
}

const EST_BOARD_WIDTH = 2560;
const EST_BOARD_HEIGHT = 1440;

const RAW_STANDS: RawStandDefinition[] = [
  { label: 'A18', x: 461.851, y: 229 },
  { label: 'A19', x: 371.48, y: 229 },
  { label: 'A20', x: 281.11, y: 229 },
  { label: 'A21', x: 190.74, y: 229 },
  { label: 'A22', x: 100.37, y: 229 },
  { label: 'A23', x: 10, y: 229 },
  { label: 'A50', x: 247, y: 556 },
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
  { label: 'E26', x: 1844, y: 626.939 },
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
  { label: 'C26', x: 1300, y: 197 },
  { label: 'C28', x: 1300, y: 350 },
  { label: 'C30', x: 1301, y: 503 },
  { label: 'C32', x: 1301, y: 656 },
  { label: 'C34', x: 1301, y: 809 },
  { label: 'C36', x: 1301.24, y: 962.1 },
  { label: 'B6', x: 895, y: 642 },
  { label: 'B8', x: 895, y: 795 },
  { label: 'B10', x: 896, y: 948 },
  { label: 'B19', x: 892, y: 1101.42 },
  { label: 'B16', x: 802, y: 1104 },
  { label: 'B17', x: 982.76, y: 1101.42 },
  { label: 'B5', x: 988, y: 593.313 },
  { label: 'B3', x: 988, y: 440 },
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

export default function D1Stand({ isOpen, onClose, stand, onRequest }: D1StandProps) {
  const [selectedStand, setSelectedStand] = useState<string>(stand);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [requestError, setRequestError] = useState<string | null>(null);

  const handleStandClick = (standId: string) => {
    setSelectedStand(standId);
    setRequestError(null);
  };

  const handleRequestNewStand = async () => {
    if (!selectedStand || isSubmitting) return;
    setIsSubmitting(true);
    setRequestError(null);
    try {
      await onRequest?.(selectedStand);
      onClose();
    } catch (error) {
      setRequestError(error instanceof Error ? error.message : 'Stand request was rejected');
    } finally {
      setIsSubmitting(false);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70" onClick={onClose}>
      <div
        className="relative box-border flex aspect-[3/2] max-h-[85vh] w-[95%] flex-col overflow-hidden rounded-lg border-2 border-[#1D293D] bg-[#011328] p-[10px]"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Stand map board */}
        <div className="relative mx-[5%] h-[90%] w-[90%] box-border overflow-hidden border-[10px] border-[#1D293D] bg-[#7b7b7b]">
          {RAW_STANDS.map((standDef) => {
            const isSelected = selectedStand === standDef.label;

            const dotColor = isSelected ? 'bg-[#43C6E7]' : 'bg-[#6B7280]';
            const standPosition = {
              '--stand-left': `${(standDef.x / EST_BOARD_WIDTH) * 100}%`,
              '--stand-top': `${(standDef.y / EST_BOARD_HEIGHT) * 100}%`,
            } as CSSProperties;

            return (
              <button
                key={standDef.label}
                type="button"
                onClick={() => handleStandClick(standDef.label)}
                title={`${standDef.label} (availability checked when requested)`}
                style={standPosition}
                className={`absolute [left:var(--stand-left)] [top:var(--stand-top)] flex h-[10%] w-[3%] translate-[10%] cursor-pointer flex-col items-center justify-between rounded-md bg-[#f0f0f0] px-1 py-1.5 text-sm leading-none font-bold text-[#111] ${isSelected ? 'border-2 border-[#9be9ff]' : 'border border-[#9ea3a8]'}`}
                aria-label={`Stand ${standDef.label}`}
              >
                <span>{standDef.label}</span>
                <span className={`mb-0.5 block h-3.5 w-3.5 rounded-full ${dotColor} ${isSelected ? 'border-2 border-[#9be9ff]' : 'border border-[#0a1f2d]'}`} />
              </button>
            );
          })}
        </div>

        {requestError && <div role="alert" className="absolute bottom-[10%] left-[5%] z-10 w-[90%] bg-[#B63F3F] px-3 py-2 text-center font-bold text-white">{requestError}</div>}

        {/* Bottom controls */}
        <div className="mx-[5%] mt-[10px] flex h-[10%] w-[90%] gap-[10px]">
          <button
            onClick={handleRequestNewStand}
            disabled={!selectedStand || isSubmitting}
            className={`box-border h-full w-[61%] rounded border-[10px] border-[#1D293D] text-[clamp(12px,1.5vh,18px)] font-bold text-white transition-colors duration-200 ${!selectedStand || isSubmitting ? 'cursor-not-allowed bg-[#3a4c58]' : 'cursor-pointer bg-[#1A475F]'}`}
          >
            {isSubmitting ? 'CHECKING STAND' : 'REQUEST NEW STAND'}
          </button>

          <button
            onClick={onClose}
            className="box-border h-full w-[29%] cursor-pointer rounded border-[10px] border-[#1D293D] bg-white text-[clamp(12px,1.5vh,18px)] font-bold text-black transition-colors duration-200"
          >
            CLICK TO CANCEL
          </button>
        </div>
      </div>
    </div>
  );
}
