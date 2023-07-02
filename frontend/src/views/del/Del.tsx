import './Del.css'
import BasePlate from "../../components/BasePlate.tsx"
import InformationBarComp from '../../components/InformationBar.tsx'

function Del() {

  return (
    <>
      <div className="Fill">
          <InformationBarComp stationA="TE" stationB="TW" rwyDep="22R" rwyAar="22L" QNH={1015} atisLetter="D" atisWinds="250/17"/>
          <BasePlate />
      </div>
    </>

  )
}

export default Del