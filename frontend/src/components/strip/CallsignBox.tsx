import './strip.css'

export default function CallsignBox(props: any) {
  return (
    <>
      <div className='CallsignBox'>
          <p>{props.callsign}</p>
      </div>
    </>
  )
}