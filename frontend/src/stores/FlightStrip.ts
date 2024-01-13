import { makeAutoObservable } from 'mobx'
import { FlightStripStore } from './FlightStripStore'
import { CoordinationState, CoordinationUpdate } from '../services/models'
import client from '../services/api/StripsApi'

export class FlightStrip {
  store: FlightStripStore
  callsign: string
  aircraftType = ''
  aircraftRegistration = ''
  aircraftCategory = ''
  origin = ''
  destination = ''
  runway = ''
  clearenceLimit = ''
  stand = ''
  eobt = ''
  tsat = ''
  ctot = ''
  cleared = false
  bay = ''
  sequence: number | null = null
  route = ''
  controller: string | null = null
  nextController: string | null = null
  squawk = ''
  remarks = ''

  constructor(store: FlightStripStore, callsign: string) {
    makeAutoObservable(this, {
      store: false,
      callsign: false,
    })

    this.store = store
    this.callsign = callsign
  }

  public updateController(update: CoordinationUpdate) {
    switch (update.state) {
      case CoordinationState.Created:
        this.nextController = update.to
        break
      case CoordinationState.Accepted:
        this.controller = update.to
        this.nextController = null
        break
      case CoordinationState.Cancelled:
      case CoordinationState.Rejected:
        this.nextController = null
    }
  }

  public clear(internal = true) {
    this.cleared = true
    this.bay = 'STARTUP'
    if (internal) {
      api.setCleared(this.callsign, true)
    }
    //client.airport.moveStrip('EKCH', 'LIVE', this.callsign, { bay: 'STARUP' })
    //client.airport.upsertStrip('EKCH', 'LIVE', this.callsign, { cleared: true })
  }

  public move(bay: string) {
    this.bay = bay
  }
}
