interface MRKBTNProps {
  isMarked: boolean;
  disabled: boolean;
  onClick: () => void;
}

export default function MRKBTN({ isMarked, disabled, onClick }: MRKBTNProps) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={`text-[1.41vw] font-bold h-[calc(4.72vh-14px)] my-[7px] w-[3.52vw] flex items-center justify-center shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow)] outline-none ${
        isMarked
          ? "bg-[#FF00F5] text-black"
          : "bg-bay-btn text-white"
      } ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
    >
      MRK
    </button>
  );
}