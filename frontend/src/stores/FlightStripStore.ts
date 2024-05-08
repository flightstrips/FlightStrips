import { makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore'
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
    makeAutoObservable(this, {
      rootStore: false,
    })

    signalRService.on('CoordinationUpdate', (update) =>
      this.handleCoordinationUpdate(update),
    )
    signalRService.on('ReceiveStripUpdate', (update) =>
      this.handleStripUpdate(update),
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

  public inBay(bay: string): FlightStrip[] {
    return this.flightStrips.filter((plan) => plan.bay == bay)
  }

  public dispose() {
    signalRService.off('CoordinationUpdate', this.handleCoordinationUpdate)
    signalRService.off('StripUpdate', this.handleStripUpdate)
  }
}
