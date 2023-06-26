import { action, observable } from "mobx";
import { RootStore } from "./RootStore";
import Flightstrip from "../data/interfaces/flightstrip";
// I don't like this, should create some interfaces somewhere common instead
import { FlightDataUpdatedMessage } from "../../electron/network/euroscope/interfaces/FlightDataUpdatedMessage";




export class FlightStripStore {
    @observable flightStrips: Flightstrip[] = []
    rootStore: RootStore

    constructor(rootStore: RootStore) {
        this.rootStore = rootStore
        api.onFlightPlanUpdated((plan: FlightDataUpdatedMessage) => this.updateFlightPlanData(plan))

    }

    @action updateFlightPlanData(data: FlightDataUpdatedMessage) {
        console.log(data)
        let flightstrip = this.flightStrips.find((strip, _) => strip.callsign == data.callsign)

        if (!flightstrip) {
            flightstrip = {
                pilotCID: 0,
                pilotName: '',
                callsign: data.callsign,
                actype: data.aircraftFPType,
                acreg: '',
                accat: data.aircraftWtc.toString(),
                departingICAO: data.origin,
                destinationICAO: data.destination,
                departureRWY: data.departureRwy,
                arrivalRWY: data.arrivalRwy,
                clearancelimit: '',
                stand: '',
                eobt: parseInt(data.estimatedDeparture),
                tsat: 0,
                ctot: 0,
            }

            this.flightStrips.push(flightstrip)
            return
        }

        flightstrip.actype = data.aircraftFPType
        flightstrip.accat = data.aircraftWtc.toString()
        flightstrip.destinationICAO = data.origin
        flightstrip.destinationICAO = data.destination
        flightstrip.departureRWY = data.departureRwy
        flightstrip.arrivalRWY = data.arrivalRwy
        flightstrip.eobt = parseInt(data.estimatedDeparture)
    }
}