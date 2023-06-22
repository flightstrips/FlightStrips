import CallsignBox from './strip/CallsignBox'
import DelEOBT from './strip/DelEOBT'
import DelTSATCTOT from './strip/DelTSATCTOT'
import DestinationStand from './strip/DestinationStand'
import './strip/strip.css'

export default function Strip(props) {
  return (
    <>
        <div className='strip departure-bg'>
            <CallsignBox callsign="VKG1335"/>
            <DestinationStand desicao="LGKR" stand="D3"/>
            <DelEOBT eobt="1314"/>
            <DelTSATCTOT tsat="1234" ctot="4321"/>
        </div>
    </>
  )
}