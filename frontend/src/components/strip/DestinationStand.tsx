import './strip.css'

export default function DestinationStand(props) {
  return (
    <>
      <div className='DestinationStand'>
          {props.desicao} <br />
          {props.stand}
      </div>
    </>
  )
}