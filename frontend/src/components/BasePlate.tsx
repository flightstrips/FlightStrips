import CLRDELStrip from "./CLRDELStrip";
import Strip from "./strip";

export default function BasePlate() {
    return (
      <div className="baseplate">
        <div className="baseBay">
          <Strip />
        </div>
        <div className="baseBay">b</div>
        <div className="baseBay">c</div>
        <div className="baseBay">d</div>
      </div>
    )
  }
  