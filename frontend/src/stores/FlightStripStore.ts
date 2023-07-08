import { action, makeObservable, observable } from 'mobx'
import { RootStore } from './RootStore'
import Flightstrip from '../data/interfaces/flightstrip'
import { FlightPlanUpdate } from '../../shared/FlightPlanUpdate'

export class FlightStripStore {
  flightStrips: Flightstrip[] = []
  rootStore: RootStore

  constructor(rootStore: RootStore) {
    this.rootStore = rootStore
    makeObservable(this, {
      flightStrips: observable,
      updateFlightPlanData: action,
      setCleared: action,
    })
  }

  public setCleared(callsign: string, cleared: boolean) {
    const flightstrip = this.flightStrips.find(
      (strip) => strip.callsign == callsign,
    )
    if (!flightstrip) return

    flightstrip.cleared = cleared
  }

  public updateFlightPlanData(data: FlightPlanUpdate) {
    let flightstrip = this.flightStrips.find(
      (strip) => strip.callsign == data.callsign,
    )

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
        cleared: false,
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

  public pending(sasOnly: boolean): Flightstrip[] {
    return this.flightStrips.filter(
      (plan) =>
        plan.departingICAO == 'EKCH' &&
        !plan.cleared &&
        sasOnly === plan.callsign.toUpperCase().startsWith('SAS'),
    )
  }

  public cleared(): Flightstrip[] {
    return this.flightStrips.filter((plan) => plan.cleared)
  }

  /*
    @action clear(callsign: string) {

    }

    @action move(callsign: string, bay: number) {

    }
    */
}
