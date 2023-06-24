import './strip.css'

export default function DelEOBT(props: any) {
  return (
    <>
      <div className='DelEOBT'>
          <span>EOBT:</span> <span>{props.eobt}</span>
      </div>
    </>
  )
}