import Flightstrip from '../data/interfaces/flightstrip';
import './CLRDELStrip.css'



export default function CLRDELStrip({callsign, destinationICAO, stand,eobt,tsat,ctot}:Flightstrip) {

  return (
    <>
        <div className="baselayer">
            <div>
                {callsign}
            </div>
            <div>
                {destinationICAO}
            </div>
            <div>
                {stand}
            </div>
            <div>
                {eobt}
            </div>
            <div>
                {tsat}
            </div>
            <div>
                {ctot}
            </div>
        </div>
    </>
  )
}