import { action, makeObservable, observable } from 'mobx'
import { RootStore } from './RootStore'
import { FlightPlanUpdate } from '../../shared/FlightPlanUpdate'
import {
  CoordinationUpdate,
  StripStateEvent,
  StripUpdate,
} from '../services/models.ts'
import { signalRService } from '../services/SignalRService.ts'
import { FlightStrip } from './FlightStrip.ts'

export class FlightStripStore {
  flightStrips: FlightStrip[] = []
  rootStore: RootStore

  constructor(rootStore: RootStore) {
    this.rootStore = rootStore
    makeObservable(this, {
      rootStore: false,
      flightStrips: observable,
      updateFlightPlanData: action,
      setCleared: action,
      handleStripUpdate: action,
    })

    signalRService.on('CoordinationUpdate', this.handleCoordinationUpdate)
    signalRService.on('ReceiveStripUpdate', this.handleStripUpdate)
  }

  public setCleared(callsign: string, cleared: boolean) {
    const flightstrip = this.flightStrips.find(
      (strip) => strip.callsign == callsign,
    )
    if (!flightstrip || cleared) return

    flightstrip.clear(false)
  }

  public handleCoordinationUpdate = (update: CoordinationUpdate) => {
    const strip = this.flightStrips.find(
      (strip) => strip.callsign === update.callsign,
    )

    if (!strip) {
      return
    }

    strip.updateController(update)
  }

  public setSquawk = (callsign: string, squawk: string) => {
    const strip = this.flightStrips.find((strip) => strip.callsign === callsign)

    if (!strip) return

    strip.squawk = squawk
  }

  public handleStripUpdate = (update: StripUpdate) => {
    const strip = this.flightStrips.find(
      (strip) => strip.callsign === update.callsign,
    )

    if (update.eventState == StripStateEvent.Created) {
      // TODO create strip if not exist
    }

    if (!strip) {
      console.log(`Did not find strip ${update.callsign}!`)
      return
    }

    console.log(`Found strip ${update.callsign}: ${JSON.stringify(update)}!`)

    switch (update.eventState) {
      case StripStateEvent.Created:
      case StripStateEvent.Updated:
        strip.bay = update.bay.toLowerCase()
        strip.cleared = update.cleared
        strip.controller = update.positionFrequency
        strip.sequence = update.sequence
        break
      case StripStateEvent.Deleted:
        this.flightStrips.splice(this.flightStrips.indexOf(strip), 1)
        break
    }
  }

  public async updateFlightPlanData(data: FlightPlanUpdate) {
    let flightstrip = this.flightStrips.find(
      (strip) => strip.callsign == data.callsign,
    )

    if (!flightstrip) {
      flightstrip = new FlightStrip(this, data.callsign)
      flightstrip.stand = 'A7'
      flightstrip.bay = this.getBay(data.callsign, false, data.origin)
      flightstrip.route = data.route
      this.flightStrips.push(flightstrip)
    }

    flightstrip.aircraftType = data.aircraftFPType
    flightstrip.aircraftCategory = data.aircraftWtc.toString()
    flightstrip.origin = data.origin
    flightstrip.destination = data.destination
    flightstrip.runway = data.departureRwy
    flightstrip.eobt = data.estimatedDepartureTime
    flightstrip.remarks = data.remarks
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

  public inBay(bay: string): FlightStrip[] {
    return this.flightStrips.filter((plan) => plan.bay == bay)
  }

  public dispose() {
    signalRService.off('CoordinationUpdate', this.handleCoordinationUpdate)
    signalRService.off('StripUpdate', this.handleStripUpdate)
  }
}
