import { EOBT } from './strip/eobt'
import { FSCS } from './strip/fscs'
import { DESSTD } from './strip/desstd'
import { TSATCTOT } from './strip/tsatctot'
import { OwnerBOX } from './strip/ownerbox'

export function FlightStrip(props: any) {
  if (props.clearanceGranted) {
    return (
      <>
        <div className="flex border-white border-x-4 border-y-2 w-fit h-16 bg-[#BEF5EF] text-black">
          <OwnerBOX />
          <FSCS cs={props.cs} />
          <DESSTD des={props.des} stand={props.stand} />
          <EOBT time={props.time} />
          <TSATCTOT TSAT={props.TSAT} />
        </div>
      </>
    )
  } else {
    return (
      <>
        <div className="flex border-white border-x-4 border-y-2 w-fit h-16 bg-[#BEF5EF] text-black">
          <FSCS cs={props.cs} />
          <DESSTD des={props.des} stand={props.stand} />
          <EOBT time={props.time} />
          <TSATCTOT TSAT={props.TSAT} />
        </div>
      </>
    )
  }
}
