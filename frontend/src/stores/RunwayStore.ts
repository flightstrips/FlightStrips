import { makeAutoObservable } from 'mobx'
import { ActiveRunway } from '../../shared/ActiveRunway'
import { RootStore } from './RootStore'

export class RunwayStore {
  rootStore: RootStore
  departure = ''
  arrival = ''

  constructor(root: RootStore) {
    this.rootStore = root
    makeAutoObservable(this, {
      rootStore: false,
    })
    api.onActiveRunways((runways) => this.setActiveRunways(runways))
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
