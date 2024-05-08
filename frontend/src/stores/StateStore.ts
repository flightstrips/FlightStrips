import { makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore.ts'
import { ControllerPosition } from '../data/models.ts'
import { signalRService } from '../services/SignalRService.ts'

export class StateStore {
  rootStore: RootStore
  connectedToEuroScope = false
  connectedToBackend = false
  controller: string = ControllerPosition.Unknown
  overrideView: string | null = null
  session = 'NONE'

  constructor(root: RootStore) {
    makeAutoObservable(this, {
      rootStore: false,
    })
    this.rootStore = root

    this.checkConnectionToBackend()
    setInterval(() => this.checkConnectionToBackend(), 10000)
  }

  public checkConnectionToBackend() {
    const state = signalRService.getState()

    if (state === 'Connected') {
      this.connectedToBackend = true
      return
    } else this.connectedToBackend = false

    if (state == 'Connecting') return

    signalRService.tryReconnect()
  }

  public handleEuroScopeConnectionUpdate(isConnected: boolean) {
    this.connectedToEuroScope = isConnected
  }

  public setOverrideView(view: string | null) {
    this.overrideView = view
  }

  public setController(controller: ControllerPosition, callsign: string) {
    this.controller = controller

    if (
      //this.vatsimConnection === 0 ||
      this.controller == ControllerPosition.Unknown
    )
      return

    signalRService.subscribe(this.session, callsign, this.controller)
  }

  get isReady() {
    return true
  }

  get view() {
    if (this.overrideView !== null) return this.overrideView
    switch (this.controller) {
      case ControllerPosition.EKCH_DEL:
      case ControllerPosition.EKDK_CTR:
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
    if (this.controller === ControllerPosition.Unknown)
      return 'Identifying position...'
    if (!this.isReady) return `Unknown position ${this.controller}...`
    return 'Postion identified switching view...'
  }
}
