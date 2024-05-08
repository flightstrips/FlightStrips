import { makeAutoObservable } from 'mobx'
import { FlightStripStore } from './FlightStripStore'
import {
  CoordinationState,
  CoordinationUpdate,
  StripUpdate,
} from '../services/models'
import client from '../services/api/StripsApi'

export class FlightStrip {
  store: FlightStripStore
  isSynced = false
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
  sid = ''
  fl = ''
  reg = ''
  hdg = ''
  alt = 'FL070'
  deice = ''

  constructor(store: FlightStripStore, callsign: string) {
    makeAutoObservable(this, {
      store: false,
      callsign: false,
      isSynced: false,
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

  public handleBackendUpdate(update: StripUpdate) {
    this.bay = update.bay
    this.cleared = update.cleared
    this.controller = update.positionFrequency
    this.sequence = update.sequence
  }

  public clear(isCleared = true, internal = true) {
    if (this.cleared) {
      return
    }

    this.cleared = isCleared
    this.bay = 'STARTUP'
    if (internal) {
      //api.setCleared(this.callsign, isCleared)
    }

    client.airport.clearStrip(
      'EKCH',
      this.store.rootStore.stateStore.session,
      this.callsign,
      {
        isCleared,
      },
    )
  }

  public move(bay: string) {
    this.bay = bay

    client.airport.moveStrip(
      'EKCH',
      this.store.rootStore.stateStore.session,
      this.callsign,
      { bay: bay },
    )
  }

  get nitosRemarks() {
    return ''
  }

  get callsignIncludingCommunicationType() {
    return this.callsign
    /*
    switch (this.communicationType) {
      case CommunicationType.Unknown:
      case CommunicationType.Voice:
        return this.callsign
      case CommunicationType.Text:
        return `${this.callsign}/t`
      case CommunicationType.Receive:
        return `${this.callsign}/r`
    }
    */
  }
}
