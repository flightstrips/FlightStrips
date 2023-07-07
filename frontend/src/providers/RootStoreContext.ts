import { createContext, useContext } from 'react'
import { RootStore } from '../stores/RootStore'

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

export function getRoot() {
  const root = store ?? initializeStore()
  return root
}

function initializeStore(): RootStore {
  const s = new RootStore()
  api.onFlightPlanUpdated((plan) =>
    s.flightStripStore.updateFlightPlanData(plan),
  )
  api.onSetCleared((callsign, cleared) =>
    s.flightStripStore.setCleared(callsign, cleared),
  )

  return s
}
