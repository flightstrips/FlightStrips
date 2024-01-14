// eslint-disable-next-line react-refresh/only-export-components
export function FSCS(props: { Callsign: string }) {
  return (
    <div className="flex text-xl font-bold items-center pl-2 w-28 border-2 border-[#85B4AF]">
      {props.Callsign}
    </div>
  )
}
