import Flightstrip from '../data/interfaces/flightstrip';
import './CLRDELStrip.css'



export default function CLRDELStrip({callsign, destinationICAO, stand,eobt,tsat,ctot}:Flightstrip) {

  return (
    <>
        <div className="baselayer dep">
            <div className='callsign'>
                {callsign}
            </div>
            <div className='destStand'>
                <span className='destinationICAO'>{destinationICAO}</span><br />
                <span className='stand'>{stand}</span>
            </div>
            <div>
                
            </div>
            <div>
                EOBT {eobt}
            </div>
            <div>
                TSAT: {tsat}<br />
                CTOT: {ctot}
            </div>
        </div>
    </>
  )
}