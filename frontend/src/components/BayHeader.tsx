export default function BayHeader(props: {
  title: string
  msg?: boolean
  buttons?: JSX.Element
}) {
  const msg = props.msg ?? false

  return (
    <div
      className={`${
        msg ? 'bg-[#285A5C]' : 'bg-slate-800'
      } 'w-full h-12  text-white font-bold flex items-center justify-between'`}
    >
      <p className="ml-2 text-xl uppercase">{props.title}</p>
      <div className="flex flex-row">{props.buttons}</div>
    </div>
  )
}
