import './strip.css'

export default function DestinationStand(props: any) {
  return (
    <>
      <div className='DestinationStand'>
          {props.desicao} <br />
          {props.stand}
      </div>
    </>
  )
}