import { makeAutoObservable } from 'mobx'
import { FlightStripStore } from './FlightStripStore'
import {
  CoordinationState,
  CoordinationUpdate,
  StripUpdate,
} from '../services/models'
import client from '../services/api/StripsApi'
import { StripResponseModel } from '../services/api/generated/FlightStripsClient'

export class FlightStrip {
  store: FlightStripStore
  callsign: string
  aircraftType = ''
  aircraftRegistration = ''
  aircraftCategory = ''
  origin = ''
  destination = ''
  alternate = ''
  runway = ''
  clearenceLimit = ''
  stand = ''
  tobt = ''
  tsat = ''
  ctot = ''
  eobt = ''
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
  alt = 7000
  deice = ''

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

  public setData(data: StripResponseModel) {
    this.origin = data.origin ?? ''
    this.destination = data.destination ?? ''
    this.alternate = data.alternate ?? ''
    this.remarks = data.remarks ?? ''
    this.squawk = data.assignedSquawk ?? ''
    this.sid = data.sid ?? ''
    this.alt = data.clearedAltitude ?? 7000
    this.aircraftCategory = data.aircraftCategory ?? ''
    this.aircraftType = data.aircraftType ?? ''
    this.runway = data.runway ?? ''
    // communication type
    //this.c
    this.stand = data.stand ?? ''
    this.tobt = data.tobt ?? ''
    this.tsat = data.tsat ?? ''
    this.cleared = data.cleared ?? false
    this.controller = data.controller ?? null
    this.sequence = data.sequence ?? null
    this.bay = data.bay
  }

  public handleBackendUpdate(update: StripUpdate) {
    this.origin = update.origin
    this.destination = update.destination
    this.alternate = update.alternate
    this.remarks = update.remarks
    this.squawk = update.assignedSquawk
    this.sid = update.sid ?? ''
    this.alt = update.clearedAltitude ?? 7000
    this.aircraftCategory = update.aircraftCategory
    this.aircraftType = update.aircraftType
    this.runway = update.runway
    // communication type
    //this.c
    this.stand = update.stand
    this.tobt = update.tobt
    this.tsat = update.tsat ?? ''
    this.cleared = update.cleared
    this.controller = update.controller
    this.sequence = update.sequence
    this.bay = update.bay
  }

  public getClearedAlt() {
    // TODO for other airports
    if (this.alt > 5000) {
      return `FL${this.alt / 100}`
    }

    return `${this.alt} ft`
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
