interface MessageProps {
  children: React.ReactNode;
  from?: string;
}

export function Message({ children, from }: MessageProps) {
  return (
    <div className="bg-[#2a2a2a] border-b border-[#444] text-white text-sm px-2 py-1">
      {from && <span className="text-[#aaa] text-xs mr-2">[{from}]</span>}
      {children}
    </div>
  );
}
