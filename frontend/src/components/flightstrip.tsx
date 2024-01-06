import { OwnerBOX } from './strip/ownerbox'
import Flightstrip from '../data/interfaces/flightstrip.ts'
import { observer } from 'mobx-react'

const FlightStrip = observer((props: { strip: Flightstrip }) => {
  return (
    <>
      <div className="w-[90%] min-w-96 h-14 bg-[#BEF5EF] border-l-4 border-r-4 border-t-2 border-b-2 mb-1 flex items-center ">
        {props.strip.cleared && <OwnerBOX />}
        <div className="w-[30%] border-r-2 border-l-2 border-t-2 border-b-2 border-[#85B4AF] max-w-64 h-full flex items-center pl-2 text-base font-medium">
          {props.strip.callsign}
        </div>
        <div className="w-[20%] border-r-1 border-l-1 border-t-2 border-b-2 border-[#85B4AF] pl-4 pr-4 h-full flex flex-col  items-center justify-center text-center">
          <span className="font-bold">{props.strip.destinationICAO}</span>
          <span className="font-bold">{props.strip.stand}</span>
        </div>
        <div className="w-[25%] border-r-1 border-l-1 border-t-2 border-b-2 border-[#85B4AF] h-full flex items-top justify-between text-center pl-2 pr-2 whitespace-nowrap">
          <div>EOBT</div>
          <div>{props.strip.eobt}</div>
        </div>
        <div className="w-[25%] border-r-2 border-l-1 border-t-2 border-b-2 border-[#85B4AF] h-full flex flex-col ">
          <span className="border-[#85B4AF] border-b-1 border-r-1 text-left flex items-center w-full h-full pl-2 pr-8 whitespace-nowrap">
            TSAT: {props.strip.tsat}
          </span>
          <div className="border-[#85B4AF] border-t-1 border-r-1 text-left flex items-center w-full h-full pl-2 pr-8 whitespace-nowrap">
            CTOT: {props.strip.tsat}
          </div>
        </div>
      </div>
    </>
  )
})

export { FlightStrip }
