import { makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore.ts'
import { ConnectionType } from '../../shared/ConnectionType.ts'
import { ControllerPosition } from '../data/models.ts'

export class StateStore {
  rootStore: RootStore
  connectedToEuroScope = false
  vatsimConnection = ConnectionType.Disconnected
  connectedToBackend = true // TODO
  controller = ControllerPosition.Unknown

  constructor(root: RootStore) {
    makeAutoObservable(this, {
      rootStore: false,
    })
    this.rootStore = root
  }

  public handleEuroScopeConnectionUpdate(isConnected: boolean) {
    this.connectedToEuroScope = isConnected
  }

  public handleVatsimConnectionUpdate(connection: ConnectionType) {
    this.vatsimConnection = connection
    if (connection === ConnectionType.Disconnected) {
      this.rootStore.flightStripStore.reset()
      this.rootStore.controllerStore.reest()
    }
  }

  public setController(controller: ControllerPosition) {
    this.controller = controller
  }

  get isReady() {
    return (
      this.connectedToBackend &&
      this.connectedToEuroScope &&
      this.vatsimConnection !== ConnectionType.Disconnected &&
      this.controller !== null &&
      this.view !== '/'
    )
  }

  get view() {
    switch (this.controller) {
      case ControllerPosition.EKCH_DEL:
        return '/ekch/del'
      case ControllerPosition.EKCH_A_GND:
      case ControllerPosition.EKCH_D_GND:
        return '/ekch/gnd'
      case ControllerPosition.EKCH_C_TWR:
      case ControllerPosition.EKCH_GE_TWR:
        return '/ekch/ctwr'
      case ControllerPosition.EKCH_A_TWR:
      case ControllerPosition.EKCH_D_TWR:
        return '/ekch/twr'
      default:
        return '/'
    }
  }

  get loadingLabel() {
    if (!this.connectedToBackend) return 'Waiting for connection to server...'
    if (!this.connectedToEuroScope)
      return 'Waiting for connection to EuroScope...'
    if (this.vatsimConnection == ConnectionType.Disconnected)
      return 'Waiting for connection to Vatsim...'
    if (this.controller === ControllerPosition.Unknown)
      return 'Identifying position...'
    if (!this.isReady) return `Unknown position ${this.controller}...`
    return 'Postion identified switching view...'
  }
}
