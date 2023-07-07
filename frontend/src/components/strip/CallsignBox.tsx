import './strip.css'

export default function CallsignBox(props: { callsign: string }) {
  return (
    <>
      <div className="CallsignBox">
        <p>{props.callsign}</p>
      </div>
    </>
  )
}
