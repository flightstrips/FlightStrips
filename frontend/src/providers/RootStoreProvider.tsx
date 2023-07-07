import { ReactNode } from 'react'
import { StoreContext, getRoot } from './RootStoreContext'

export function RootStoreProvider({ children }: { children: ReactNode }) {
  // only create root store once (store is a singleton)
  const root = getRoot()

  return <StoreContext.Provider value={root}>{children}</StoreContext.Provider>
}
