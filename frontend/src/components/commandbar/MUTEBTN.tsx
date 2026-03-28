interface MUTEBTNProps {
  muted: boolean;
  onClick: () => void;
}

export default function MUTEBTN({ muted, onClick }: MUTEBTNProps) {
  return (
    <button
      onClick={onClick}
      className={`text-2xl font-bold h-[calc(4.72vh-14px)] my-[7px] w-[80px] flex items-center justify-center shadow-[inset_2px_0_0_#d3d3d3,_inset_0_2px_0_#d3d3d3] outline-none ${
        muted ? "bg-[#FF4444] text-white" : "bg-[#646464] text-white"
      }`}
    >
      {muted ? "MUTE" : "SND"}
    </button>
  );
}
