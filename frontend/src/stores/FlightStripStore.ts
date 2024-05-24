import { makeAutoObservable, runInAction } from 'mobx'
import { RootStore } from './RootStore'
import { CoordinationUpdate, StripUpdate } from '../services/models.ts'
import { signalRService } from '../services/SignalRService.ts'
import { FlightStrip } from './FlightStrip.ts'
import client from '../services/api/StripsApi.ts'

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

  public async loadStrips() {
    const strips = await client.airport.listStrips(
      'EKCH',
      this.rootStore.stateStore.session,
    )

    runInAction(() => {
      this.flightStrips = strips.data.map((s) => {
        const strip = new FlightStrip(this, s.callsign)
        strip.setData(s)
        return strip
      })
    })
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

    if (!strip) {
      // new strip
      const strip = new FlightStrip(this, update.callsign)
      strip.handleBackendUpdate(update)
      this.flightStrips.push(strip)
      return
    }

    strip.handleBackendUpdate(update)
  }

  public inBay(bay: string): FlightStrip[] {
    return this.flightStrips.filter((plan) => plan.bay == bay)
  }

  public dispose() {
    signalRService.off('CoordinationUpdate', this.handleCoordinationUpdate)
    signalRService.off('StripUpdate', this.handleStripUpdate)
  }
}
