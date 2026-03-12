// Tailwind class constants (hex must be literal strings for JIT)
const CLS_WRAPPER = "bg-[#2a2a2a] border-b border-[#444] text-white text-sm px-2 py-1";
const CLS_FROM    = "text-[#aaa] text-xs mr-2";

interface MessageProps {
  children: React.ReactNode;
  from?: string;
}

export function Message({ children, from }: MessageProps) {
  return (
    <div className={CLS_WRAPPER}>
      {from && <span className={CLS_FROM}>[{from}]</span>}
      {children}
    </div>
  );
}
