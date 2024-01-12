import { observer } from 'mobx-react'
import { FlightStrip } from '../stores/FlightStrip'
import './CLRDELStrip.css'

export const CLRDELStrip = observer((props: { strip: FlightStrip }) => {
  return (
    <>
      <div className="baselayer dep">
        <div className="callsign">{props.strip.callsign}</div>
        <div className="destStand">
          <span className="destinationICAO">{props.strip.destination}</span>
          <br />
          <span className="stand">{props.strip.stand}</span>
        </div>
        <div></div>
        <div>EOBT {props.strip.eobt}</div>
        <div>
          TSAT: {props.strip.tsat}
          <br />
          CTOT: {props.strip.ctot}
        </div>
      </div>
    </>
  )
})
