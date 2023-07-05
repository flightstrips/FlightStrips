import './strip.css'

export default function DelTSATCTOT(props: { tsat: string, ctot: string }) {
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