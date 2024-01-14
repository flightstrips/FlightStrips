import React from 'react'
interface LayoutProps {
  Title: string
  msg?: boolean
  Buttons?: JSX.Element
}

export default function BayHeader(props: LayoutProps) {
  const msg = props.msg ?? false
  return (
    <div
      className={`${
        msg ? 'bg-[#285a5c]' : 'bg-header-grey'
      } 'w-full h-10 text-white text-xl flex items-center pl-2 pr-2 justify-between font-semibold'`}
    >
      <p className="uppercase">{props.title}</p>
      <div className="flex flex-row">{props.buttons}</div>
    </div>
  )
}
