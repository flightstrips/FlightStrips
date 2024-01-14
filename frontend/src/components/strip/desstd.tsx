// eslint-disable-next-line react-refresh/only-export-components
export function DESSTD(props: { Stand: string; DestinationICAO: string }) {
  return (
    <div className="flex font-bold items-center w-20 border-2 border-[#85B4AF] flex-col">
      <span className="text-xl ">{props.DestinationICAO}</span>
      <span>{props.Stand}</span>
    </div>
  )
}
