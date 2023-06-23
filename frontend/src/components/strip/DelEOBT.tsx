import './strip.css'

export default function DelEOBT(props) {
  return (
    <>
      <div className='DelEOBT'>
          <span>EOBT:</span> <span>{props.eobt}</span>
      </div>
    </>
  )
}