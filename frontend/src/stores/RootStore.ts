import { FlightStripStore } from './FlightStripStore'

export class RootStore {
  flightStripStore: FlightStripStore

  constructor() {
    this.flightStripStore = new FlightStripStore(this)
  }
}
