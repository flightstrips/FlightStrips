import { Volume2, VolumeX } from "lucide-react";

interface MUTEBTNProps {
  muted: boolean;
  onClick: () => void;
}

export default function MUTEBTN({ muted, onClick }: MUTEBTNProps) {
  return (
    <button
      onClick={onClick}
      className={`h-[calc(4.72vh-14px)] my-[7px] w-[3.52vw] flex items-center justify-center shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow)] outline-none ${
        muted ? "bg-[#FF4444] text-white" : "bg-bay-btn text-white"
      }`}
    >
      {muted
        ? <VolumeX className="w-[1.6vw] h-[1.6vw]" />
        : <Volume2 className="w-[1.6vw] h-[1.6vw]" />
      }
    </button>
  );
}
