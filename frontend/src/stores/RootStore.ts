import { createContext } from "react"
import { FlightStripStore } from "./FlightStripStore"

export class RootStore {
    flightStripStore: FlightStripStore

    constructor() {
        this.flightStripStore = new FlightStripStore(this)
    }
}

export const StoreContext = createContext(new RootStore())