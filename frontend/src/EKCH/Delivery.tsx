import { CommandBar } from '../components/commandbar'
import { FlightStrip } from '../components/flightstrip'
import { NewFlightBtn } from '../components/headerbuttons/newflightbtn'
import { PlannedFlightBtn } from '../components/headerbuttons/plannedflightbtn'
import BayHeader from '../components/BayHeader'

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

function Delivery() {
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
        {flights.map((item) => (
          <FlightStrip
            key={item.callsign}
            cs={item.callsign}
            des={item.destination}
            stand={item.parking}
            time={item.EOBT}
            TSAT={item.TSAT}
          />
        ))}
      </div>
      <div className="w-full bg-bay-grey">
        <BayHeader title="SAS" />
        {flights_sas.map((item) => (
          <FlightStrip
            key={item.callsign}
            cs={item.callsign}
            des={item.destination}
            stand={item.parking}
            time={item.EOBT}
            TSAT={item.TSAT}
          />
        ))}
      </div>
      <div className="w-full bg-bay-grey flex flex-col">
        <div className="h-2/3">
          <BayHeader title="Cleared" />
          <FlightStrip
            cs="SAS1206"
            des="ENGM"
            stand="B10"
            time="13:39"
            TSAT="14:00"
            clearanceGranted
          />
        </div>
        <div className="justify-self-end">
          <BayHeader title="Messages" msg />
        </div>
      </div>
      <div className="w-full bg-bay-grey">
        <BayHeader title="Standby" />
      </div>

      <CommandBar />
    </div>
  )
}

export default Delivery
