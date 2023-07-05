import './strip.css'

export default function DestinationStand(props: { desicao: string, stand: string }) {
  return (
    <>
      <div className='DestinationStand'>
          {props.desicao} <br />
          {props.stand}
      </div>
    </>
  )
}