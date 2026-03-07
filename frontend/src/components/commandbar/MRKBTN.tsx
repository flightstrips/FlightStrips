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
      className={`text-xl font-bold p-2 border-2 ${
        isMarked
          ? "bg-[#FF00F5] text-black"
          : "bg-[#646464] text-white"
      } ${disabled ? "opacity-50 cursor-not-allowed" : ""}`}
    >
      MRK
    </button>
  );
}