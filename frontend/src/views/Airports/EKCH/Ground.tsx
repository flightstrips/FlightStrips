import BayHeader from '../../../components/BayHeader'
import { Planned } from '../../../components/Buttons/Planned'
import { ControllerMessages } from '../../../components/Buttons/ControllerMessages'
import { MemoryAid } from '../../../components/Buttons/MemoryAid'
import { CommandBar } from '../../../components/commandbar'
import {
  useFlightStripStore,
  useStateStore,
} from '../../../providers/RootStoreContext.ts'
import { observer } from 'mobx-react'
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { FlightStrip } from '../../../components/flightstrip.tsx'

const Ground = observer(() => {
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
        <div className="h-1/4">
          <BayHeader title="MESSAGES" msg buttons={<ControllerMessages />} />
        </div>
        <div className="h-1/3">
          <BayHeader title="FINAL" />
          {flightStripStore.inBay('FINAL').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/3">
          <BayHeader title="RWY ARR" />
          {flightStripStore.inBay('RWY ARR').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-64">
          <BayHeader title="STAND" />
          {flightStripStore.inBay('STAND').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
      </div>
      <div className="w-full bg-bay-grey border-l-[4px] border-r-[4px]">
        <div className="h-3/5">
          <BayHeader title="TWY DEP" buttons={<MemoryAid />} />
          {flightStripStore.inBay('TWYDEP').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
        <div className="h-1/3">
          <BayHeader title="TWY ARR" buttons={<MemoryAid />} />
          {flightStripStore.inBay('TWYARR').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
      </div>
      <div className="w-full bg-bay-grey flex flex-col border-l-[4px] border-r-[4px]">
        <div className="h-3/6">
          <BayHeader title="STARTUP" />
          <div className="h-[calc(100%-2.5rem)] overflow-scroll overflow-x-hidden border-[2px] border-[#555355]">
            {flightStripStore.inBay('STARTUP').map((item) => (
              <FlightStrip strip={item} key={item.callsign} />
            ))}
          </div>
        </div>
        <div>
          <BayHeader title="PUSH BACK" />
          {flightStripStore.inBay('PUSHBACK').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
      </div>
      <div className="w-full bg-bay-grey border-l-[4px] border-r-[4px]">
        <div className="h-4/5">
          <BayHeader title="CLR DEL" buttons={<Planned />} />
          <div className="h-[calc(100%-2.5rem)] overflow-scroll overflow-x-hidden border-[2px] border-[#555355]">
            {flightStripStore.inBay('OTHER').map((item) => (
              <FlightStrip strip={item} key={item.callsign} />
            ))}
          </div>
        </div>
        <div>
          <BayHeader title="DE-ICE" />
          {flightStripStore.inBay('DEICE').map((item) => (
            <FlightStrip strip={item} key={item.callsign} />
          ))}
        </div>
      </div>

      <CommandBar />
    </div>
  )
})

export default Ground
