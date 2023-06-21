import InformationBar from "../data/informationbar.ts"

interface InformationBarProps {
  InformationBar: InformationBar[];
}

export default function InformationBarComp(props) {
	const a = props.stationA
  const b = props.stationB
  const rwyDep = props.rwyDep
  return (
    <div className="InformationBar">
      <div className="curStation">
          {a} + {b}
      </div>
      <div className="rwyDep">
          {rwyDep}
      </div>
    </div>
  )
}