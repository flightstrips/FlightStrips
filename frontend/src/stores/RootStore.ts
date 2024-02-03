import { ControllerStore } from './ControllerStore.ts'
import { FlightStripStore } from './FlightStripStore.ts'
import { RunwayStore } from './RunwayStore.ts'
import { StateStore } from './StateStore.ts'

export class RootStore {
  flightStripStore: FlightStripStore
  stateStore: StateStore
  controllerStore: ControllerStore
  runwayStore: RunwayStore

  constructor() {
    this.flightStripStore = new FlightStripStore(this)
    this.stateStore = new StateStore(this)
    this.controllerStore = new ControllerStore(this)
    this.runwayStore = new RunwayStore(this)
    api.ready()
  }
}
