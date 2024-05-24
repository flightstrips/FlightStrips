import { makeAutoObservable, runInAction } from 'mobx'
import { RootStore } from './RootStore'
import { Controller } from './Controller'
import { signalRService } from '../services/SignalRService'
import client from '../services/api/StripsApi'

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

  public async getControllers(session: string) {
    const response = await client.airport.listOnlinePositions('EKCH', session, {
      connected: true,
    })

    if (response.error) {
      console.log('Got error from server: ', response.error.detail)
      return
    }

    runInAction(() => {
      this.controllers = response.data
        .filter((r) => r.frequency !== undefined)
        .map((r) => {
          const controller = new Controller(r.position ?? 'Unknown controller')
          if (r.frequency) {
            controller.frequency = r.frequency
          }
          return controller
        })
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
}
