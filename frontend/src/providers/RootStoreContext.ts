import { createContext, useContext } from 'react'
import { RootStore } from '../stores/RootStore.ts'

let store: RootStore
export const StoreContext = createContext<RootStore | undefined>(undefined)
StoreContext.displayName = 'StoreContext'

export function useRootStore() {
  const context = useContext(StoreContext)
  if (context === undefined) {
    throw new Error('useRootStore must be used within RootStoreProvider')
  }

  return context
}

export function useFlightStripStore() {
  const { flightStripStore } = useRootStore()
  return flightStripStore
}

export function useStateStore() {
  const { stateStore } = useRootStore()
  return stateStore
}

export function useRunwayStore() {
  const { runwayStore } = useRootStore()
  return runwayStore
}

export function getRoot() {
  const root = store ?? initializeStore()
  return root
}

function initializeStore(): RootStore {
  const s = new RootStore()
  api.onMe((callsign) => s.controllerStore.setMe(callsign))
  api.onNavitage((route) => s.stateStore.setOverrideView(route))
  return s
}
