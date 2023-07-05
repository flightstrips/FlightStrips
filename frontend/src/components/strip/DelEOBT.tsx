import './strip.css'

export default function DelEOBT(props: { eobt: string }) {
  return (
    <>
      <div className='DelEOBT'>
          <span>EOBT:</span> <span>{props.eobt}</span>
      </div>
    </>
  )
}