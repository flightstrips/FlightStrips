interface LayoutProps {
  title: string
  message?: boolean
  information?: boolean
  buttons?: JSX.Element
}

export default function BayHeader(props: LayoutProps) {
  if (props.information) {
    return (
      <div className="bg-[#b3b3b3] w-full h-10 text-[#393939] text-xl flex items-center pl-2 pr-2 justify-between font-semibold">
        <p className="uppercase">{props.title}</p>
        <div className="flex flex-row">{props.buttons}</div>
      </div>
    )
  }
  if (props.message) {
    return (
      <div className="bg-[#285a5c] w-full h-10 text-white text-xl flex items-center pl-2 pr-2 justify-between font-semibold">
        <p className="uppercase">{props.title}</p>
        <div className="flex flex-row">{props.buttons}</div>
      </div>
    )
  } else {
    return (
      <div className="bg-header-grey w-full h-10 text-white text-xl flex items-center pl-2 pr-2 justify-between font-semibold">
        <p className="uppercase">{props.title}</p>
        <div className="flex flex-row">{props.buttons}</div>
      </div>
    )
  }
}
