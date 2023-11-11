export function DESSTD(props: any) {
  return (
    <div className="flex font-bold items-center w-20 border-2 border-[#85B4AF] flex-col">
      <span className="text-xl ">{props.des}</span>
      <span>{props.stand}</span>
    </div>
  )
}
