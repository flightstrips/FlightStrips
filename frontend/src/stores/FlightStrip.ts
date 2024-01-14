import { action, makeAutoObservable } from 'mobx'
import { FlightStripStore } from './FlightStripStore'
import { CoordinationState, CoordinationUpdate } from '../services/models'
import client from '../services/api/StripsApi'
import { FlightPlanUpdate } from '../../shared/FlightPlanUpdate'
import { StripState } from '../services/api/generated/FlightStripsClient'

const BACKEND = false

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

  public handleUpdateFromEuroScope(update: FlightPlanUpdate) {
    if (!this.isSynced && BACKEND) {
      this.isSynced = true
      client.airport
        .getStrip('EKCH', 'LIVE', update.callsign)
        .then(
          action('GotStrip', (response) => {
            const data = response.data
            this.bay = data.bay
            this.sequence = data.sequence ?? null
            if (data.cleared !== undefined && this.cleared !== data.cleared) {
              this.cleared = data.cleared
            }
          }),
        )
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        .catch((_) => {
          client.airport.upsertStrip('EKCH', 'LIVE', update.callsign, {
            cleared: false,
            destination: update.destination,
            origin: update.origin,
            state: StripState.None,
          })
        })
    }

    this.aircraftType = update.aircraftFPType
    this.aircraftCategory = update.aircraftWtc.toString()
    this.origin = update.origin
    this.destination = update.destination
    this.runway = update.departureRwy
    this.eobt = update.estimatedDepartureTime
    this.remarks = update.remarks
    this.route = update.route
    this.sid = update.sidName
    this.fl = (update.finalAltitude / 100).toString()

    const index = update.remarks.toUpperCase().indexOf('REG/')
    if (index !== -1) {
      this.reg = update.remarks.substring(index + 4, index + 9)
    }
  }

  public clear(internal = true) {
    if (this.cleared) {
      return
    }

    this.cleared = true
    this.bay = 'STARTUP'
    if (internal) {
      api.setCleared(this.callsign, true)
    }

    if (BACKEND) {
      client.airport.moveStrip('EKCH', 'LIVE', this.callsign, {
        bay: 'STARTUP',
      })
      client.airport.upsertStrip('EKCH', 'LIVE', this.callsign, {
        cleared: true,
        destination: this.destination,
        origin: this.origin,
      })
    }
  }

  public move(bay: string) {
    this.bay = bay

    if (BACKEND) {
      client.airport.moveStrip('EKCH', 'LIVE', this.callsign, { bay: bay })
    }
  }

  get nitosRemarks() {
    return ''
  }
}
