import './strip.css'

export default function DelTSATCTOT(props: { tsat: number, ctot: number }) {
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