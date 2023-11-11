export function TSATCTOT(props: any) {
  return (
    <div className="flex flex-col justify-between">
      <div className="flex w-32 border-b-[1px] pl-2 border-t-2 border-r-2 border-l-2 h-8 border-[#85B4AF]">
        TSAT {props.TSAT}
      </div>
      <div className="flex w-32 border-t-[1px] pl-2 border-b-2 border-r-2 border-l-2 h-8 border-[#85B4AF]">
        CTOT
      </div>
    </div>
  )
}
