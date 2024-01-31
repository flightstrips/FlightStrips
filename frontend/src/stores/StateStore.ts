import { action, makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore.ts'
import { ConnectionType } from '../../shared/ConnectionType.ts'
import { ControllerPosition } from '../data/models.ts'
import { signalRService } from '../services/SignalRService.ts'

export class StateStore {
  rootStore: RootStore
  connectedToEuroScope = false
  vatsimConnection = ConnectionType.Disconnected
  connectedToBackend = false
  controller: string = ControllerPosition.Unknown
  overrideView: string | null = null
  session = 'NONE'

  constructor(root: RootStore) {
    makeAutoObservable(this, {
      rootStore: false,
    })
    this.rootStore = root

    api.onEuroScopeConnectionUpdate((isConnected) =>
      this.handleEuroScopeConnectionUpdate(isConnected),
    )
    api.onVatsimConnectionUpdate((connection) =>
      this.handleVatsimConnectionUpdate(connection),
    )

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

  public handleVatsimConnectionUpdate(connection: ConnectionType) {
    if (this.vatsimConnection === connection) return
    if (connection === ConnectionType.Disconnected) {
      if (this.session !== 'NONE') {
        signalRService.unsubscribe(this.session, this.controller)
      }
      this.rootStore.flightStripStore.reset()
      this.rootStore.controllerStore.reest()
      this.overrideView = null
      this.controller = ControllerPosition.Unknown
    }

    const prev = this.vatsimConnection

    this.vatsimConnection = connection
    this.setSession()

    if (prev === ConnectionType.Disconnected && this.session !== 'NONE') {
      signalRService.subscribeAirport(this.session)
    }
  }

  public setOverrideView(view: string | null) {
    this.overrideView = view
  }

  public setController(controller: ControllerPosition, callsign: string) {
    this.controller = controller

    if (
      this.vatsimConnection === 0 ||
      this.controller == ControllerPosition.Unknown
    )
      return

    signalRService.subscribe(this.session, callsign, this.controller)
  }

  private setSession() {
    switch (this.vatsimConnection) {
      case ConnectionType.Disconnected:
      case ConnectionType.Proxy:
        this.session = 'NONE'
        break
      case ConnectionType.Client:
      case ConnectionType.Simulator:
      case ConnectionType.Sweatbox:
        this.session = 'SWEATBOX'
        break
      case ConnectionType.Playback:
        this.session = 'PLAYBACK-' + Math.random().toString(36).slice(2, 7)
        break
      case ConnectionType.Direct:
        this.session = 'LIVE'
        break
    }
  }

  get isReady() {
    return (
      this.overrideView !== null ||
      (this.connectedToBackend &&
        this.connectedToEuroScope &&
        this.vatsimConnection !== ConnectionType.Disconnected &&
        this.controller !== null &&
        this.view !== '/')
    )
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
    if (this.vatsimConnection == ConnectionType.Disconnected)
      return 'Waiting for connection to Vatsim...'
    if (this.controller === ControllerPosition.Unknown)
      return 'Identifying position...'
    if (!this.isReady) return `Unknown position ${this.controller}...`
    return 'Postion identified switching view...'
  }
}
