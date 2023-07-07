import './strip.css'

export default function DelEOBT(props: { eobt: number }) {
  return (
    <>
      <div className="DelEOBT">
        <span>EOBT:</span> <span>{props.eobt}</span>
      </div>
    </>
  )
}
