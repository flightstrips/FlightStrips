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
      className={`text-[1.41vw] font-bold h-[3.42vh] my-[7px] w-[3.52vw] flex items-center justify-center shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow)] outline-none ${
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
