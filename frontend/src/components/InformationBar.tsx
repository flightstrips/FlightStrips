import './InformationBar.css'
import ATIS from './ATIS'

export default function InformationBarComp(props: any) {

  return (
    <div className="InformationBar">
      <div className="curStation">
          {props.stationA} + {props.stationB}
      </div>
      <div className="rwyDep">
          <p>DEP</p>
          <div className="rwyBox">
            {props.rwyDep}
          </div>
      </div>
      <div className="rwyAar">
          <p>AAR</p>
          <div className="rwyBox">
            {props.rwyAar}
          </div>
      </div>
      <div className="QNH">
          {props.QNH}
      </div>
      <button className="atis">
        <ATIS />
      </button>
      <div className="atisLetter">
          <ATIS />
      </div>
      <div className="atisWinds">
          {props.atisWinds}
      </div>
    </div>
  )
}