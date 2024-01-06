import { action, makeObservable, observable } from 'mobx'
import { RootStore } from './RootStore'
import Flightstrip from '../data/interfaces/flightstrip'
import { FlightPlanUpdate } from '../../shared/FlightPlanUpdate'
import {
  CoordinationState,
  CoordinationUpdate,
  StripStateEvent,
  StripUpdate,
} from '../services/models.ts'
import { signalRService } from '../services/SignalRService.ts'
import stripsService from '../services/StripsService.ts'
import { StripState } from '../services/api/generated/FlightStripsClient.ts'

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

    signalRService.on('CoordinationUpdate', this.handleCoordinationUpdate)
    signalRService.on('StripUpdate', this.handleStripUpdate)
  }

  public setCleared(callsign: string, cleared: boolean) {
    const flightstrip = this.flightStrips.find(
      (strip) => strip.callsign == callsign,
    )
    if (!flightstrip) return

    flightstrip.bay = this.getBay(callsign, cleared, flightstrip.departingICAO)
    flightstrip.cleared = cleared
  }

  public handleCoordinationUpdate(update: CoordinationUpdate) {
    const index = this.flightStrips.findIndex(
      (strip) => strip.callsign === update.callsign,
    )

    if (index === -1) {
      return
    }

    switch (update.state) {
      case CoordinationState.Created:
        this.flightStrips[index] = {
          ...this.flightStrips[index],
          nextController: update.to,
        }
        break
      case CoordinationState.Accepted:
        this.flightStrips[index] = {
          ...this.flightStrips[index],
          controller: update.to,
          nextController: null,
        }
        break
      case CoordinationState.Cancelled:
      case CoordinationState.Rejected:
        this.flightStrips[index] = {
          ...this.flightStrips[index],
          nextController: null,
        }
    }
  }

  public handleStripUpdate(update: StripUpdate) {
    const index = this.flightStrips.findIndex(
      (strip) => strip.callsign === update.callsign,
    )

    switch (update.state) {
      case StripStateEvent.Created:
      case StripStateEvent.Updated:
        if (index !== -1) {
          this.flightStrips[index] = {
            ...this.flightStrips[index],
            bay: update.bay,
            cleared: update.cleared,
            controller: update.positionFrequency,
            sequence: update.sequence,
          }
        }
        break

      case StripStateEvent.Deleted:
        // Remove a flight strip
        if (index !== -1) {
          this.flightStrips.splice(index, 1)
        }
        break
    }
  }

  public async updateFlightPlanData(data: FlightPlanUpdate) {
    let flightstrip = this.flightStrips.find(
      (strip) => strip.callsign == data.callsign,
    )

    if (!flightstrip) {
      flightstrip = {
        pilotCID: 0,
        callsign: data.callsign,
        actype: data.aircraftFPType,
        acreg: '',
        accat: data.aircraftWtc.toString(),
        departingICAO: data.origin,
        destinationICAO: data.destination,
        departureRWY: data.departureRwy,
        arrivalRWY: data.arrivalRwy,
        clearancelimit: '',
        stand: 'A7',
        eobt: parseInt(data.estimatedDeparture) || 1200,
        tsat: 1200,
        ctot: 1200,
        cleared: false,
        bay: this.getBay(data.callsign, false, data.origin),
        controller: null,
        nextController: null,
        sequence: 0,
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

  // TODO remove
  private getBay(callsign: string, isCleared: boolean, origin: string): string {
    const upper = callsign.toUpperCase()

    if (origin.toUpperCase() !== 'EKCH') return 'arr'

    if (isCleared) {
      return 'cleared'
    }

    if (upper.startsWith('SAS')) {
      return 'sas'
    }

    if (upper.startsWith('NOZ')) {
      return 'norwegian'
    }

    return 'other'
  }

  public inBay(bay: string): Flightstrip[] {
    return this.flightStrips.filter((plan) => plan.bay == bay)
  }

  public dispose() {
    signalRService.off('CoordinationUpdate', this.handleCoordinationUpdate)
    signalRService.off('StripUpdate', this.handleStripUpdate)
  }
}
