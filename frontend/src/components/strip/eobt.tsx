import React from 'react' // we need this to make JSX compile

export function EOBT(props: any) {
  return (
    <div className="flex  items-top w-40 pl-4 pr-4 border-2 border-[#85B4AF] text-xl justify-between">
      <div className="flex">EBOT</div>
      <div className="flex">{props.time}</div>
    </div>
  )
}
