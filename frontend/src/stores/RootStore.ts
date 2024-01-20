import { ControllerStore } from './ControllerStore.ts'
import { FlightStripStore } from './FlightStripStore.ts'
import { StateStore } from './StateStore.ts'

export class RootStore {
  flightStripStore: FlightStripStore
  stateStore: StateStore
  controllerStore: ControllerStore

  constructor() {
    this.flightStripStore = new FlightStripStore(this)
    this.stateStore = new StateStore(this)
    this.controllerStore = new ControllerStore(this)
    api.ready()
  }
}
