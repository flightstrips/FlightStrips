import { makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore'
import { Controller } from './Controller'
import { ControllerUpdate } from '../../shared/ControllerUpdate'
import { ControllerPosition } from '../data/models'

export class ControllerStore {
  rootStore: RootStore
  me?: Controller
  controllers: Controller[] = []

  constructor(rootStore: RootStore) {
    this.rootStore = rootStore
    makeAutoObservable(this, {
      rootStore: false,
    })
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

  public handleControllerUpdate(update: ControllerUpdate) {
    if (this.me?.callsign === update.callsign) {
      if (update.frequency !== this.me.frequency) {
        this.rootStore.stateStore.setController(
          update.frequency as ControllerPosition,
        )
      }

      this.me.frequency = update.frequency
      this.me.position = update.postion
    }

    let controller = this.controllers.find((c) => c.callsign == update.callsign)

    if (!controller) {
      controller = new Controller(update.callsign)
      this.controllers.push(controller)
    }

    controller.frequency = update.frequency
    controller.position = update.postion
  }

  public handleControllerDisconnect(disconnect: ControllerUpdate) {
    const controller = this.controllers.find(
      (c) => c.callsign == disconnect.callsign,
    )

    if (!controller) return

    this.controllers.splice(this.controllers.indexOf(controller), 1)
  }
}
