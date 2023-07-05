import Strip from "./strip";
import BayHeader from "./BayHeader";
import { observer } from "mobx-react";
import { useFlightStripStore } from "../providers/RootStoreProvider";

export const BasePlate = observer(function BasePlate() {
  const flightStripStore = useFlightStripStore()

  return (
    <div className="baseplate">
      <div className="baseBay">
        <BayHeader name="others" showNewButton={true} showPlannedButton={true}/>

        {flightStripStore.flightStrips.map(plan => {
          return (<Strip plan={plan} />)
        })}
      </div>
      <div className="baseBay">
        <BayHeader name="SAS"/>
      </div>
      <div className="baseBay">c</div>
      <div className="baseBay">d</div>
    </div>
  )
})
  