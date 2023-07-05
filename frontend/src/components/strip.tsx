import Flightstrip from '../data/interfaces/flightstrip'
import CallsignBox from './strip/CallsignBox'
import DelEOBT from './strip/DelEOBT'
import DelTSATCTOT from './strip/DelTSATCTOT'
import DestinationStand from './strip/DestinationStand'
import './strip/strip.css'

export default function Strip(props: { plan: Flightstrip }) {
  return (
    <>
        <div className='strip departure-bg'>
            <CallsignBox callsign={props.plan.callsign} />
            <DestinationStand desicao={props.plan.departingICAO} stand={props.plan.stand} />
            <DelEOBT eobt={props.plan.eobt} />
            <DelTSATCTOT tsat={props.plan.tsat} ctot={props.plan.ctot}/>
        </div>
    </>
  )
}