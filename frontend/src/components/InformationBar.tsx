import './InformationBar.css'

export default function InformationBarComp(props: any) {
	const a = props.stationA
  const b = props.stationB
  return (
    <div className="InformationBar">
      <div className="curStation">
          {a} + {b}
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
        ATIS
      </button>
      <div className="atisLetter">
          {props.atisLetter}
      </div>
      <div className="atisWinds">
          {props.atisWinds}
      </div>
    </div>
  )
}