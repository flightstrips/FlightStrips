interface Flightstrip {
  pilotCID: number
  callsign: string
  actype: string
  acreg: string
  accat: string
  departingICAO: string
  destinationICAO: string
  departureRWY: string
  arrivalRWY: string
  clearancelimit: string
  stand: string | null
  eobt: number
  tsat: number
  ctot: number
  cleared: boolean
  bay: string
  controller: string | null
  nextController: string | null
  sequence: number | null
}

export default Flightstrip
