// eslint-disable-next-line react-refresh/only-export-components
export function EOBT(props: { EOBT: string }) {
  return (
    <div className="flex items-top w-40 pl-4 pr-4 border-2 border-[#85B4AF] text-xl justify-between">
      <div className="flex">EBOT</div>
      <div className="flex">{props.EOBT}</div>
    </div>
  )
}
