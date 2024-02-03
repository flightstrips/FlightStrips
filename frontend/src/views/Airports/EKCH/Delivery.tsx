import BayHeader from '../../../components/BayHeader.tsx'
import { Planned } from '../../../components/Buttons/Planned.tsx'
import { ControllerMessages } from '../../../components/Buttons/ControllerMessages.tsx'
import { CommandBar } from '../../../components/commandbar.tsx'
import { FlightStrip } from '../../../components/flightstrip.tsx'
import {
  useFlightStripStore,
  useStateStore,
} from '../../../providers/RootStoreContext.ts'
import { observer } from 'mobx-react'
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'

const Delivery = observer(() => {
  const flightStripStore = useFlightStripStore()
  const stateStore = useStateStore()
  const navigate = useNavigate()

  useEffect(() => {
    if (stateStore.view !== '/ekch/del') {
      navigate(stateStore.view)
    }
  }, [navigate, stateStore.view])

  return (
    <>
      <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center shrink">
        <div className="bg-[#555355] w-full h-auto border-l-[4px] border-r-[2px]">
          <BayHeader title="OTHERS" buttons={<Planned />} />
          <div className="h-[calc(100%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('OTHER').map((item) => (
              <FlightStrip strip={item} key={item.callsign} />
            ))}
          </div>
        </div>
        <div className="bg-[#555355] h-full w-full border-l-[4px] border-r-[4px]">
          <BayHeader title="SAS" />
          <div className="h-[calc(60%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('SAS').map((item) => (
              <FlightStrip strip={item} key={item.callsign} />
            ))}
          </div>
          <BayHeader title="NORWEGIAN" />
          <div className="h-[calc(40%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('NORWEGIAN').map((item) => (
              <FlightStrip strip={item} key={item.callsign} />
            ))}
          </div>
        </div>
        <div className="bg-[#555355] w-full h-auto border-l-[4px] border-r-[4px]">
          <BayHeader title="CLEARED" />
          <div className="h-[calc(50%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('STARTUP').map((item) => (
              <FlightStrip strip={item} key={item.callsign} />
            ))}
          </div>
          <BayHeader
            title="MESSAGES"
            message
            buttons={<ControllerMessages />}
          />
          <div className="h-[calc(33%-2.5rem)] overflow-auto overflow-x-hidden"></div>
        </div>
        <div className="bg-[#555355] w-full h-auto border-l-[4px] border-r-[4px]">
          <BayHeader title="PUSHBACK" />
          <div className="h-[calc(33%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('PUSHBACK').map((item) => (
              <FlightStrip strip={item} />
            ))}
          </div>
          <BayHeader title="TWY DEP" information />
          <div className="h-[calc(66%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('TWY DEP').map((item) => (
              <FlightStrip strip={item} />
            ))}
          </div>
        </div>
      </div>
      <CommandBar />
    </>
  )
})

export default Delivery
