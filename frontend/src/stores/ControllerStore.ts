import { makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore'
import { Controller } from './Controller'
import { signalRService } from '../services/SignalRService'

export class ControllerStore {
  rootStore: RootStore
  me?: Controller
  controllers: Controller[] = []

  constructor(rootStore: RootStore) {
    this.rootStore = rootStore
    makeAutoObservable(this, {
      rootStore: false,
    })

    signalRService.on('ReceiveControllerSectorsUpdate', (update) =>
      console.log(update),
    )
  }

  public reest() {
    this.controllers = []
  }

  public setMe(callsign: string) {
    if (!this.me) {
      this.me = new Controller(callsign)
    } else {
      this.me.callsign = callsign
    }
  }
}
