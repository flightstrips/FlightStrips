import BayHeader from '../../components/BayHeader.tsx'
import { CommandBar } from '../../components/commandbar.tsx'
import { FlightStrip } from '../../components/flightstrip.tsx'
import { useFlightStripStore } from '../../providers/RootStoreContext.ts'
import { observer } from 'mobx-react'

const Delivery = observer(() => {
  const flightStripStore = useFlightStripStore()

  return (
    <>
      <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center shrink">
        <div className="bg-[#555355] w-full h-auto border-r-4 border-[#a9a9a9]">
          <BayHeader title="OTHERS" />
          <div className="h-[calc(100%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('OTHER').map((item) => (
              <FlightStrip strip={item} key={item.callsign} />
            ))}
          </div>
        </div>
        <div className="bg-[#555355] h-full w-full border-l-4 border-r-4 border-[#a9a9a9]">
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
        <div className="bg-[#555355] w-full h-auto border-l-4 border-r-4 border-[#a9a9a9]">
          <BayHeader title="CLEARED" />
          <div className="h-[calc(50%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('cleared').map((item) => (
              <FlightStrip strip={item} />
            ))}
          </div>
          <BayHeader title="MESSAGES" msg />
          <div className="h-[calc(33%-2.5rem)] overflow-auto overflow-x-hidden"></div>
        </div>
        <div className="bg-[#555355] w-full h-auto border-l-2 border-[#a9a9a9]">
          <BayHeader title="PUSHBACK" />
          <div className="h-[calc(33%-2.5rem)] overflow-auto overflow-x-hidden">
            {flightStripStore.inBay('PUSHBACK').map((item) => (
              <FlightStrip strip={item} />
            ))}
          </div>
          <BayHeader title="TWY DEP" />
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
