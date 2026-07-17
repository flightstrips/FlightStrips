import GSX from '../../../assets/efb/GSX.png';

interface D1DownloadsDialogProps {
  isOpen: boolean;
  onClose: () => void;
}

export default function D1DownloadsDialog({ isOpen, onClose }: D1DownloadsDialogProps) {
  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-[1000] flex items-center justify-center bg-black/70"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-label="Downloads"
        className="relative flex aspect-[3/2] max-h-[85vh] w-[95%] items-center justify-center overflow-hidden rounded-lg border-2 border-[#011328] bg-[#000109]"
        onClick={(event) => event.stopPropagation()}
      >
        <button
          type="button"
          aria-label="Close downloads"
          onClick={onClose}
          className="absolute top-[2%] right-[2%] z-10 h-14 w-14 cursor-pointer rounded-md border-0 bg-[#1A475F] text-lg font-bold text-white"
        >
          X
        </button>

        <div className="flex h-full w-full items-center justify-center gap-[3%]">
          <a
            href="https://www.flightsim.to"
            target="_blank"
            rel="noreferrer"
            aria-label="Download GSX profile"
            className="flex aspect-square w-[20%] items-center justify-center rounded-lg border-2 border-[#1A475F] bg-[#011328]"
          >
            <img src={GSX} alt="GSX" className="h-[85%] w-[85%] object-contain" />
          </a>

          <a
            href="https://www.flightsim.to"
            target="_blank"
            rel="noreferrer"
            className="flex aspect-square w-[20%] items-center justify-center rounded-lg border-2 border-[#1A475F] bg-[#011328] text-[clamp(18px,3vw,48px)] font-bold text-white no-underline"
          >
            EFB
          </a>
        </div>
      </div>
    </div>
  );
}
