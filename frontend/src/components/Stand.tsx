type StandProps = {
  label: string
  position: string
  status?: 'active' | 'reserved' | 'blue' | 'default'
  style?: React.CSSProperties
}

export default function Stand({ label, position, status = 'default' }: StandProps) {
  const baseStyle = "flex items-center justify-center text-xs font-semibold rounded transition w-full h-full"
  const colorMap = {
    default: "bg-gray-200 text-black",
    active: "bg-yellow-400 text-black",
    reserved: "bg-blue-600 text-white",
    blue: "bg-blue-500 text-white",
  }

  return (
    <div className={`${baseStyle} ${colorMap[status]} ${position}`} style={{ gridColumn: position, gridRow: position }}>
      {label}
    </div>
  )
}
