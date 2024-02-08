import BayHeader from '../../../components/BayHeader'
import { NewVFR } from '../../../components/Buttons/NewVFR'
import { ControllerMessages } from '../../../components/Buttons/ControllerMessages'
import { CommandBar } from '../../../components/commandbar'
import {
  useFlightStripStore,
  useStateStore,
} from '../../../providers/RootStoreContext.ts'
import { observer } from 'mobx-react'
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { FlightStrip } from '../../../components/flightstrip.tsx'

const Tower = observer(() => {
  const flightStripStore = useFlightStripStore()
  const stateStore = useStateStore()
  const navigate = useNavigate()

  useEffect(() => {
    if (!stateStore.isReady) {
      navigate('/')
    }
  }, [navigate, stateStore.isReady, stateStore.view])

  return (
    <div className="bg-background-grey h-screen w-screen flex justify-center">
      <div className="w-full bg-bay-grey flex flex-col border-l-[4px] border-r-[4px]">
        <div className="h-2/5">
          <BayHeader title="FINAL" />
          {flightStripStore.inBay('FINAL').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/4">
          <BayHeader title="RWY ARR" />
          {flightStripStore.inBay('RWY ARR').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/3">
          <BayHeader title="RWY ARR" />
          {flightStripStore.inBay('RWY ARR').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
      </div>

      <div className="w-full bg-bay-grey border-l-[4px] border-r-[4px]">
        <div className="h-2/5">
          <BayHeader title="TWY DEP" />
          {flightStripStore.inBay('TWY DEP').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/4">
          <BayHeader title="RWY ARR" />
          {flightStripStore.inBay('RWY ARR').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/3">
          <BayHeader title="AIRBORNE" />
          {flightStripStore.inBay('AIRBORNE').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
      </div>

      <div className="w-full bg-bay-grey flex flex-col border-l-[4px] border-r-[4px]">
        <div className="h-2/5">
          <BayHeader title="CONTROL ZONE" buttons={<NewVFR />} />
          {flightStripStore.inBay('CONTROL ZONE').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/4">
          <BayHeader title="PUSH BACK" />
          {flightStripStore.inBay('PUSH BACK').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-2/6">
          <BayHeader
            title="MESSAGES"
            message
            buttons={<ControllerMessages />}
          />
        </div>
      </div>

      <div className="w-full bg-bay-grey border-l-[4px] border-r-[4px]">
        <div className="h-3/5">
          <BayHeader title="CLR DEL" />
          {flightStripStore.inBay('CLR DEL').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/5">
          <BayHeader title="DE-ICE" />
          {flightStripStore.inBay('DEICE').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/5">
          <BayHeader title="STAND" />
          {flightStripStore.inBay('STAND').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
      </div>

      <CommandBar />
    </div>
  )
})

export default Tower
