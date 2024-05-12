import { makeAutoObservable, runInAction } from 'mobx'
import { RootStore } from './RootStore.ts'
import { ControllerPosition } from '../data/models.ts'
import { signalRService } from '../services/SignalRService.ts'
import client from '../services/api/StripsApi.ts'

export interface Session {
  name: string
  airport: string
}

export interface Controller {
  name: string
}

export class StateStore {
  rootStore: RootStore
  connectedToEuroScope = false
  connectedToBackend = false
  controller: string = ControllerPosition.Unknown
  overrideView: string | null = null
  session = 'NONE'
  availableSessions: Session[] = []
  callsign = 'Unknown'
  ready = false

  constructor(root: RootStore) {
    makeAutoObservable(this, {
      rootStore: false,
    })
    this.rootStore = root

    //this.checkConnectionToBackend()
    //setInterval(() => this.checkConnectionToBackend(), 10000)
  }

  public async loadSessions() {
    const response = await client.sessions.getSessions()

    if (response.error) {
      console.log('Failed to load sessions: ', response.error)
    }

    runInAction(() => {
      if (!response.data.sessions) {
        return
      }

      this.availableSessions = response.data.sessions.map((s) => {
        return {
          name: s.name ?? 'Unknown',
          airport: 'EKCH',
        }
      })
    })
  }

  public setSession(session: string) {
    if (this.session === session) return

    this.session = session
    this.rootStore.controllerStore.getControllers(this.session)
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

  public setOverrideView(view: string | null) {
    this.overrideView = view
  }

  public async setController(callsign: string) {
    const frequency = await signalRService.subscribe(this.session, callsign)
    this.controller = frequency as ControllerPosition
    this.rootStore.flightStripStore.loadStrips()
    this.ready = true
  }

  get isReady() {
    return this.ready
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
