interface Flightstrip {
    pilotCID: number
    pilotName: string,
    callsign: string,
    actype: string,
    acreg: string,
    accat: string,
    departingICAO: string,
    destinationICAO: string,
    departureRWY: string,
    arrivalRWY: string,
    clearancelimit: string,
    stand: string,
    eobt: number,
    tsat: number,
    ctot: number,
}

export default Flightstrip;