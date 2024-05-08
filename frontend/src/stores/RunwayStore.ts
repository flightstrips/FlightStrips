import { makeAutoObservable } from 'mobx'
import { RootStore } from './RootStore'

interface ActiveRunway {
  name: string
  isDeparture: boolean
}

export class RunwayStore {
  rootStore: RootStore
  departure = ''
  arrival = ''

  constructor(root: RootStore) {
    this.rootStore = root
    makeAutoObservable(this, {
      rootStore: false,
    })
  }

  public setActiveRunways(runways: ActiveRunway[]) {
    this.departure =
      runways.find(
        (r) =>
          r.isDeparture && (r.name.startsWith('22') || r.name.startsWith('04')),
      )?.name ??
      runways.find((r) => r.isDeparture)?.name ??
      ''
    this.arrival =
      runways.find(
        (r) =>
          !r.isDeparture &&
          (r.name.startsWith('22') || r.name.startsWith('04')),
      )?.name ??
      runways.find((r) => !r.isDeparture)?.name ??
      ''
  }
}
