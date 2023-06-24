import './baseHeader.css'

function NEWButton(props: any) {
    const Enabled = props.enabled
    if (Enabled == true) {
        return <button>NEW</button>;
    }
}
function PLANNEDButton(props : any) {
    const Enabled = props.enabled
    if (Enabled == true) {
        return <button>PLANNED</button>;
    }
}


export default function BayHeader(props: any) {
    return (
      <div className="BayHeader">
        <span className='Name'>
            {props.name}
        </span>
        <span className='Buttons'>
            <NEWButton enabled={props.NEWButton}/>
            <PLANNEDButton enabled={props.PLANNEDButton}/>
        </span>
      </div>
    )
  }
  