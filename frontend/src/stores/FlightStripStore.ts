import { makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore'
import { FlightPlanUpdate } from '../../shared/FlightPlanUpdate'
import {
  CoordinationUpdate,
  StripStateEvent,
  StripUpdate,
} from '../services/models.ts'
import { signalRService } from '../services/SignalRService.ts'
import { FlightStrip } from './FlightStrip.ts'
import { CommunicationType } from '../../shared/CommunicationType.ts'

export class FlightStripStore {
  flightStrips: FlightStrip[] = []
  rootStore: RootStore

  constructor(rootStore: RootStore) {
    this.rootStore = rootStore
    makeAutoObservable(this, {
      rootStore: false,
    })

    signalRService.on('CoordinationUpdate', (update) =>
      this.handleCoordinationUpdate(update),
    )
    signalRService.on('ReceiveStripUpdate', (update) =>
      this.handleStripUpdate(update),
    )
    api.onFlightPlanUpdated((plan) => this.updateFlightPlanData(plan))
    api.onSetCleared((callsign, cleared) => this.setCleared(callsign, cleared))
    api.onSetSquawk((callsign, squawk) => this.setSquawk(callsign, squawk))
    api.onSetCommunicationType((callsign, communicationType) =>
      this.handleCommunicationTypeUpdate(callsign, communicationType),
    )
  }

  public reset() {
    this.flightStrips = []
  }

  public setCleared(callsign: string, cleared: boolean) {
    const flightstrip = this.flightStrips.find(
      (strip) => strip.callsign == callsign,
    )
    if (!flightstrip || !cleared) return

    flightstrip.clear(cleared, false)
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
        strip.handleBackendUpdate(update)
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
      this.flightStrips.push(flightstrip)
    }

    flightstrip.handleUpdateFromEuroScope(data)
  }

  public handleCommunicationTypeUpdate(
    callsign: string,
    communicationType: CommunicationType,
  ) {
    let flightstrip = this.flightStrips.find(
      (strip) => strip.callsign == callsign,
    )

    if (!flightstrip) {
      flightstrip = new FlightStrip(this, callsign)
      this.flightStrips.push(flightstrip)
    }

    flightstrip.handleCommunicationTypeUpdate(communicationType)
  }

  public inBay(bay: string): FlightStrip[] {
    return this.flightStrips.filter((plan) => plan.bay == bay)
  }

  public dispose() {
    signalRService.off('CoordinationUpdate', this.handleCoordinationUpdate)
    signalRService.off('StripUpdate', this.handleStripUpdate)
  }
}
