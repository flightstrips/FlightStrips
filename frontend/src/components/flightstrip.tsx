import { OwnerBOX } from './strip/ownerbox.tsx'
import { observer } from 'mobx-react'
import { Button } from '@nextui-org/react'
import * as Model from '../stores/FlightStrip.ts'
import { CLX } from './Buttons/CLX.tsx'

const FlightStrip = observer((props: { strip: Model.FlightStrip }) => {
  return (
    <>
      <div className="w-full h-[50px] flex items-center mb-[2px]">
        {props.strip.cleared && <OwnerBOX />}
        <Button
          radius="none"
          className="w-[26%] h-full flex items-center pl-2 text-left text-base font-medium border-t-2 border-l-2 border-b-2  border-white p-0 m-0 bg-[#bef5ef]"
        >
          {props.strip.cleared && (
            <div className="border-1 border-[#85B4AF] border-t-2 border-l-2 border-b-2 h-full w-full flex items-center pl-1 text-xl">
              {props.strip.callsignIncludingCommunicationType}
            </div>
          )}
          {!props.strip.cleared && (
            <div className="border-1 border-[#85B4AF] border-t-2 border-l-2 border-b-2 h-full w-full pl-1 text-xl">
              {props.strip.callsignIncludingCommunicationType}
            </div>
          )}
        </Button>
        <CLX Flightstrip={props.strip} />
        <div className="w-[17%] h-full text-[13px] border-t-2 border-b-2 border-white bg-[#bef5ef]">
          <span className="text-left flex items-center w-full h-1/2 text-sm border-1 border-t-2 border-[#85B4AF] flex justify-between">
            <span className="pl-1">EOBT</span>
            <span className="pr-1">{props.strip.eobt}</span>
          </span>
          <div className="text-left flex items-center w-full h-1/2 text-sm border-1 border-b-2 border-[#85B4AF] pl-1">
            CTOT: {props.strip.tsat}
          </div>
        </div>

        <div className="w-[17%] h-full border-t-2 border-b-2 border-r-2 border-white bg-[#bef5ef]">
          <span className="text-left flex items-center w-full h-1/2 text-sm border-1  border-t-2 border-[#85B4AF] pl-1">
            TOBT: {props.strip.tsat}
          </span>
          <div className="text-left flex items-center w-full h-1/2 text-sm border-1  border-b-2 border-[#85B4AF] pl-1">
            TSAT: {props.strip.tsat}
          </div>
        </div>
        {!props.strip.cleared && (
          <div className="w-fit h-full bg-[#555355]"></div>
        )}
        {props.strip.cleared && (
          <div className="w-fit h-full bg-[#555355]"></div>
        )}
      </div>
    </>
  )
})

export { FlightStrip }
