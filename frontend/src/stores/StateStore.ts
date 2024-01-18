import { makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore.ts'
import { ConnectionType } from '../../shared/ConnectionType.ts'

export class StateStore {
  rootStore: RootStore
  connectedToEuroScope = false
  vatsimConnection = ConnectionType.Disconnected
  connectedToBackend = true // TODO
  identified = false

  constructor(root: RootStore) {
    makeAutoObservable(this, {
      rootStore: false,
    })
    this.rootStore = root
  }

  public handleEuroScopeConnectionUpdate(isConnected: boolean) {
    this.connectedToEuroScope = isConnected
    console.log(`EuroScope connection ${isConnected}`)
  }

  public handleVatsimConnectionUpdate(connection: ConnectionType) {
    this.vatsimConnection = connection
  }

  public setIdentified(identified: boolean) {
    this.identified = identified
  }

  get isReady() {
    return (
      this.connectedToBackend &&
      this.connectedToEuroScope &&
      this.vatsimConnection &&
      this.identified
    )
  }

  get loadingLabel() {
    if (!this.connectedToBackend) return 'Waiting for connection to server...'
    if (!this.connectedToEuroScope)
      return 'Waiting for connection to EuroScope...'
    if (!this.vatsimConnection) return 'Waiting for connection to Vatsim...'
    if (!this.identified) return 'Identifying position...'
    return 'Postion identified switching view...'
  }
}
