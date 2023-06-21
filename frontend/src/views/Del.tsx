import './del.css'
import BasePlate from "../components/BasePlate.tsx"
import InformationBarComp from '../components/InformationBar.tsx'

function Del() {
  return (
    <div className="Fill">
        <InformationBarComp stationA="TE" stationB="TW"/>
        <BasePlate />
    </div>
  )
}

export default Del