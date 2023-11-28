import { CommandBar } from '../components/commandbar'
import { FlightStrip } from '../components/flightstrip'
import { NewFlightBtn } from '../components/headerbuttons/newflightbtn'
import { PlannedFlightBtn } from '../components/headerbuttons/plannedflightbtn'
import BayHeader from '../components/BayHeader'
import { useFlightStripStore } from '../providers/RootStoreContext.ts'
import { observer } from 'mobx-react'

const Delivery = observer(() => {
  const flightStripStore = useFlightStripStore()

  return (
    <div className="bg-background-grey h-screen w-screen flex gap-2 justify-center">
      <div className="w-full bg-bay-grey">
        <BayHeader
          title="OTHERS"
          buttons={
            <>
              <NewFlightBtn />
              <PlannedFlightBtn />
            </>
          }
        />
        {flightStripStore.inBay('other').map((item) => (
          <FlightStrip strip={item} />
        ))}
      </div>
      <div className="w-full bg-bay-grey">
        <BayHeader title="SAS" />
        {flightStripStore.inBay('sas').map((item) => (
          <FlightStrip strip={item} />
        ))}
      </div>
      <div className="w-full bg-bay-grey flex flex-col">
        <div className="h-2/3">
          <BayHeader title="Cleared" />
          {flightStripStore.inBay('cleared').map((item) => (
            <FlightStrip strip={item} />
          ))}
        </div>
        <div className="justify-self-end">
          <BayHeader title="Messages" msg />
        </div>
      </div>
      <div className="w-full bg-bay-grey">
        <BayHeader title="Standby" />
        {flightStripStore.inBay('standby').map((item) => (
          <FlightStrip strip={item} />
        ))}
      </div>

      <CommandBar />
    </div>
  )
})

export default Delivery
