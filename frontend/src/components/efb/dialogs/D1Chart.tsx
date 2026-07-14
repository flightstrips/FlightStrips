import chartsImage from '../../../assets/efb/charts-image.png';

interface D1ChartProps {
  isOpen: boolean;
  onClose: () => void;
  airport: string;
  runway: string;
  sid: string;
  arrival: boolean;
}

export default function D1Chart({ isOpen, onClose, airport, runway, sid, arrival }: D1ChartProps) {
  if (!isOpen) return null;

  const availableRunway = runway !== 'NIL' ? runway : null;
  const availableSid = !arrival && sid !== 'NIL' ? sid : null;
  const chartContext = [
    airport !== 'NIL' ? airport : null,
    availableRunway ? `runway ${availableRunway}` : null,
    availableSid ? `SID ${availableSid}` : null,
  ].filter(Boolean).join(', ');

  return (
    <div className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="efb-chart-title"
        className="relative flex aspect-[3/2] max-h-[85vh] w-[95%] overflow-hidden rounded-lg border-2 border-[#1D293D] bg-[#011328]"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex h-full w-[66.66%] items-center justify-center border-r-[10px] border-[#1D293D] bg-[#001a2e] p-[10px]">
          <img src={chartsImage} alt="Chart information" className="box-border h-full w-full border-[10px] border-[#1D293D] object-contain" />
        </div>

        <div className="flex h-full w-[33.34%] flex-col bg-[#0d2540]">
          <div className="h-[70%] overflow-auto border-b-[10px] border-[#1D293D] p-5 text-white">
            <h2 id="efb-chart-title" className="mt-0 mb-[15px] text-[clamp(16px,2.5vh,24px)] font-bold">CHARTS</h2>
            <p className="m-0 text-[clamp(12px,1.5vh,16px)] leading-[1.6] text-[#E0E0E0]">
              {chartContext
                ? `Review the current ${arrival ? 'arrival' : 'departure'} charts for ${chartContext}.`
                : `No assigned ${arrival ? 'arrival' : 'departure'} procedure is currently available.`}
            </p>
            <p className="mt-4 text-[clamp(12px,1.5vh,16px)] leading-[1.6] text-[#E0E0E0]">
              FlightStrips does not provide authoritative chart documents. Verify the current revision in your approved chart provider before use.
            </p>
          </div>

          <button type="button" className="box-border flex h-[30%] cursor-pointer items-center justify-center border-[10px] border-[#1D293D] bg-white" onClick={onClose}>
            <span className="text-[clamp(14px,2vh,20px)] font-bold text-black">CLICK TO CLOSE</span>
          </button>
        </div>

        <div className="absolute right-0 bottom-0 left-0 box-border flex h-20 items-center justify-center gap-5 border-t-[10px] border-[#1D293D] bg-[#001a2e] p-[15px]">
          <button type="button" aria-label="Previous chart" disabled className="flex h-[50px] w-[50px] items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-white text-[28px] font-bold text-black opacity-50">←</button>
          <div aria-label="Chart page 1 of 1" className="h-4 w-4 rounded-full border-2 border-white bg-white" />
          <button type="button" aria-label="Next chart" disabled className="flex h-[50px] w-[50px] items-center justify-center rounded-full border-[3px] border-[#1D293D] bg-white text-[28px] font-bold text-black opacity-50">→</button>
        </div>
      </div>
    </div>
  );
}
