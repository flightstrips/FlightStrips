import './InformationBar.css'

interface InformationBarProps {
  stationA: string
  stationB: string
  rwyDep: string
  rwyArr: string
  qnh: number
  atisWinds: string
  atisLetter: string
}

export default function InformationBarComp(props: InformationBarProps) {
  return (
    <div className="InformationBar">
      <div className="curStation">
        {props.stationA} + {props.stationB}
      </div>
      <div className="rwyDep">
        <p>DEP</p>
        <div className="rwyBox">{props.rwyDep}</div>
      </div>
      <div className="rwyArr">
        <p>ARR</p>
        <div className="rwyBox">{props.rwyArr}</div>
      </div>
      <div className="QNH">{props.qnh}</div>
      <button className="atis">A</button>
      <div className="atisLetter">A</div>
      <div className="atisWinds">{props.atisWinds}</div>
    </div>
  )
}
