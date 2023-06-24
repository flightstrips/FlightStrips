import './strip.css'

export default function DelTSATCTOT(props: any) {
  return (
    <>
      <div className='DelTSATCTOT'>
          TSAT: {props.tsat} <br />
          <hr />
          CTOT: {props.ctot}
      </div>
    </>
  )
}