import { OwnerBOX } from './strip/ownerbox'
import Flightstrip from '../data/interfaces/flightstrip.ts'
import { observer } from 'mobx-react'
import { Button } from '@nextui-org/react'
import CLX from './CLX.tsx'

const FlightStrip = observer((props: { strip: Flightstrip }) => {
  return (
    <>
      <div
        className="w-fit min-w-96 h-12 bg-[#BEF5EF] border-l-4 border-r-4 border-t-2 border-b-2 mb-1 flex items-center text-sm"
        draggable="true"
      >
        <OwnerBOX />
        <Button
          radius="none"
          className="w-24 bg-[#BEF5EF] border-r-2 border-l-2 border-t-2 border-b-2 border-[#85B4AF] max-w-64 h-full flex items-center pl-2 text-left text-base font-medium"
        >
          {props.strip.callsign}
        </Button>
        <CLX
          destinationICAO={props.strip.destinationICAO}
          stand={props.strip.stand}
          Flightstrip={props.strip}
        />
        <div className="w-[5.5rem] border-r-1 border-l-1 border-t-2 border-b-2 border-[#85B4AF] h-full flex items-top justify-between text-center pl-1 pr-1 whitespace-nowrap">
          <div>EOBT</div>
          <div>{props.strip.eobt}</div>
        </div>
        <div className="w-[5.5rem] border-r-2 border-l-1 border-t-2 border-b-2 border-[#85B4AF] h-full flex flex-col ">
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
