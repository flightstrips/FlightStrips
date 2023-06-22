import './strip.css'

export default function CallsignBox(props) {
  return (
    <>
      <div className='CallsignBox'>
          <p>{props.callsign}</p>
      </div>
    </>
  )
}