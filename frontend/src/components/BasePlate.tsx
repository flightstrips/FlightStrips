import CLRDELStrip from "./CLRDELStrip";

export default function BasePlate() {
    return (
      <div className="baseplate">
        <div className="baseBay">
          <CLRDELStrip callsign="VKG1334" destinationICAO="LGKR" stand="D3" eobt={1312} tsat={0} ctot={0}/>
        </div>
        <div className="baseBay">b</div>
        <div className="baseBay">c</div>
        <div className="baseBay">d</div>
      </div>
    )
  }
  