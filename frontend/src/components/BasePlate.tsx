import Strip from "./strip";
import BayHeader from "./BayHeader";

export default function BasePlate() {
    return (
      <div className="baseplate">
        <div className="baseBay">
          <BayHeader name="others" showNewButton={true} showPlannedButton={true}/>
          <Strip />
        </div>
        <div className="baseBay">
          <BayHeader name="SAS"/>
        </div>
        <div className="baseBay">c</div>
        <div className="baseBay">d</div>
      </div>
    )
  }
  