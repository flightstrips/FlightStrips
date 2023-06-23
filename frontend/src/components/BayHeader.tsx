import './baseHeader.css'

function NEWButton(props) {
    const Enabled = props.enabled
    if (Enabled == true) {
        return <button>NEW</button>;
    }
}
function PLANNEDButton(props) {
    const Enabled = props.enabled
    if (Enabled == true) {
        return <button>PLANNED</button>;
    }
}
function MEMAIDButton(props) {
    const Enabled = props.enabled
    if (Enabled == true) {
        return <button>MEM AID</button>;
    }
}


export default function BayHeader(props) {
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
  