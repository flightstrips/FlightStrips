import { EOBT } from './strip/eobt'
import { FSCS } from './strip/fscs'
import { DESSTD } from './strip/desstd'
import { TSATCTOT } from './strip/tsatctot'
import { OwnerBOX } from './strip/ownerbox'
import Flightstrip from '../data/interfaces/flightstrip.ts'
import { observer } from 'mobx-react'

const FlightStrip = observer((props: { strip: Flightstrip }) => {
  return (
    <>
      <div className="flex border-white border-x-4 border-y-2 w-fit h-16 bg-[#BEF5EF] text-black">
        {props.strip.cleared && <OwnerBOX />}
        <FSCS cs={props.strip.callsign} />
        <DESSTD des={props.strip.destinationICAO} stand={props.strip.stand} />
        <EOBT time={props.strip.eobt} />
        <TSATCTOT TSAT={props.strip.tsat} />
      </div>
    </>
  )
})

export { FlightStrip }
