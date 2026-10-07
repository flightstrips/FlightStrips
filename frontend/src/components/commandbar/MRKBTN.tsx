import { CLS_CMD_BEVEL } from "@/components/strip/shared";

interface MRKBTNProps {
  isMarked: boolean;
  armed: boolean;
  disabled: boolean;
  onClick: () => void;
}

export default function MRKBTN({ isMarked, armed, disabled, onClick }: MRKBTNProps) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={`text-[1.0575vw] h-[3.42dvh] my-[7px] w-[3.52vw] flex items-center justify-center ${CLS_CMD_BEVEL} outline-none ${
        isMarked
          ? "bg-[#FF00F5] text-black"
          : armed
            ? "bg-[#1BFF16] text-black"
          : "bg-bay-btn text-white"
      } ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
    >
      MRK
    </button>
  );
}
