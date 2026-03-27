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
      className={`text-2xl font-bold h-[calc(4.72vh-14px)] my-[7px] w-[80px] flex items-center justify-center shadow-[inset_2px_0_0_#d3d3d3,_inset_0_2px_0_#d3d3d3] outline-none ${
        isMarked
          ? "bg-[#FF00F5] text-black"
          : "bg-[#646464] text-white"
      } ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
    >
      MRK
    </button>
  );
}