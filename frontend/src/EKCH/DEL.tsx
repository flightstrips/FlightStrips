import React from 'react'
import { BayHeader } from '../components/bayheader'
import { CommandBar } from '../components/commandbar'
import { FlightStrip } from '../components/flightstrip'
import { NewFlightBtn } from '../components/headerbuttons/newflightbtn'
import { PlannedFlightBtn } from '../components/headerbuttons/plannedflightbtn'

const flights = [
  {
    callsign: 'NOZ1234',
    destination: 'ESSA',
    parking: 'A11',
    EOBT: '13:30',
    TSAT: '14:00',
  },
  {
    callsign: 'EWG4PJ',
    destination: 'EDDL',
    parking: 'A8',
    EOBT: '13:30',
    TSAT: '14:00',
  },
  {
    callsign: 'DLH944',
    destination: 'EDDM',
    parking: 'C35',
    EOBT: '13:30',
    TSAT: '14:00',
  },
  {
    callsign: 'EJU94KA',
    destination: 'EHAM',
    parking: 'F4',
    EOBT: '13:30',
    TSAT: '14:00',
  },
  {
    callsign: 'RYR2PW',
    destination: 'EDDN',
    parking: 'F6',
    EOBT: '13:30',
    TSAT: '14:00',
  },
]
const flights_sas = [
  {
    callsign: 'SAS632',
    destination: 'EDDL',
    parking: 'B6',
    EOBT: '13:30',
    TSAT: '14:00',
  },
  {
    callsign: 'SAS439B',
    destination: 'ESGG',
    parking: 'D2',
    EOBT: '13:30',
    TSAT: '14:00',
  },
  {
    callsign: 'SAS2787',
    destination: 'LDSP',
    parking: 'B16',
    EOBT: '13:30',
    TSAT: '14:00',
  },
]

function DEL() {
  return (
    <div className="bg-slate-400 h-full w-full flex gap-2 justify-center">
      <div className="w-full bg-gray-500">
        <BayHeader
          title="OTHERS"
          buttons={
            <>
              <NewFlightBtn />
              <PlannedFlightBtn />
            </>
          }
        />
        {flights.map((item) => (
          <FlightStrip
            cs={item.callsign}
            des={item.destination}
            stand={item.parking}
            time={item.EOBT}
            TSAT={item.TSAT}
          />
        ))}
      </div>
      <div className="w-full bg-gray-500">
        <BayHeader title="SAS" />
        {flights_sas.map((item) => (
          <FlightStrip
            cs={item.callsign}
            des={item.destination}
            stand={item.parking}
            time={item.EOBT}
            TSAT={item.TSAT}
          />
        ))}
      </div>
      <div className="w-full bg-gray-500 flex flex-col">
        <>
          <BayHeader title="Cleared" />
          <FlightStrip
            cs="SAS1206"
            des="ENGM"
            stand="B10"
            time="13:39"
            TSAT="14:00"
            clearanceGranted
          />
        </>
        <div className="justify-self-end">
          <BayHeader title="Messages" msg />
        </div>
      </div>
      <div className="w-full bg-gray-500">
        <BayHeader title="To Be Named" />
      </div>

      <CommandBar />
    </div>
  )
}

export default DEL
