import './baseHeader.css'

function NEWButton(props: { enabled: boolean }) {
  const Enabled = props.enabled
  if (Enabled == true) {
    return <button>NEW</button>
  }
}
function PLANNEDButton(props: { enabled: boolean }) {
  const Enabled = props.enabled
  if (Enabled == true) {
    return <button>PLANNED</button>
  }
}

export default function BayHeader(props: {
  name: string
  showNewButton?: boolean
  showPlannedButton?: boolean
}) {
  return (
    <div className="BayHeader">
      <span className="Name">{props.name}</span>
      <span className="Buttons">
        <NEWButton enabled={props.showNewButton ?? false} />
        <PLANNEDButton enabled={props.showPlannedButton ?? false} />
      </span>
    </div>
  )
}
