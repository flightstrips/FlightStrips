import { ipcRenderer } from 'electron'
import { FlightDataUpdatedMessage } from '../network/euroscope/interfaces/FlightDataUpdatedMessage'
import { CommunicationType } from '../../shared/CommunicationType'
import { ActiveRunway } from '../../shared/ActiveRunway'

export default {
  onFlightPlanUpdated: (handler: (plan: FlightDataUpdatedMessage) => void) => {
    ipcRenderer.on('FlightPlanUpdated', (_, args) =>
      handler(JSON.parse(args) as FlightDataUpdatedMessage),
    )
  },
  onFlightPlanDisconnect: (handler: (callsign: string) => void) => {
    ipcRenderer.on('FlightPlanDiconnect', (_, args) => handler(args))
  },
  onSetSquawk: (handler: (callsign: string, squawk: number) => void) => {
    ipcRenderer.on('SetSquawk', (_, ...args) => handler(args[0], args[1]))
  },
  onSetFinalAltitude: (
    handler: (callsign: string, altitude: number) => void,
  ) => {
    ipcRenderer.on('SetFinalAltitude', (_, ...args) =>
      handler(args[0], args[1]),
    )
  },
  onSetClearedAltitude: (
    handler: (callsign: string, altitude: number) => void,
  ) => {
    ipcRenderer.on('SetClearedAltitude', (_, ...args) =>
      handler(args[0], args[1]),
    )
  },
  onSetCommunicationType: (
    handler: (callsign: string, communication_type: CommunicationType) => void,
  ) => {
    ipcRenderer.on('SetCommunicationType', (_, ...args) =>
      handler(args[0], args[1]),
    )
  },
  onSetGroundState: (handler: (callsign: string, state: string) => void) => {
    ipcRenderer.on('SetGroundState', (_, ...args) => handler(args[0], args[1]))
  },
  onSetCleared: (handler: (callsign: string, clear: boolean) => void) => {
    ipcRenderer.on('SetCleared', (_, ...args) => handler(args[0], args[1]))
  },
  onActiveRunways: (handler: (runways: ActiveRunway[]) => void) => {
    ipcRenderer.on('OnActiveRunways', (_, args) => handler(args))
  },
  setSquawk: (callsign: string, squawk: number) => {
    ipcRenderer.send('SetSquawk', callsign, squawk)
  },
  setFinalAltitude: (callsign: string, altitude: number) => {
    ipcRenderer.send('SetFinalAltitude', callsign, altitude)
  },
  setClearedAltitude: (callsign: string, altitude: number) => {
    ipcRenderer.send('SetClearedAltitude', callsign, altitude)
  },
  setCommunicationType: (
    callsign: string,
    communication_type: CommunicationType,
  ) => {
    ipcRenderer.send('SetCommunicationType', callsign, communication_type)
  },
  setGroundState: (callsign: string, state: string) => {
    ipcRenderer.send('SetGroundState', callsign, state)
  },
  setCleared: (callsign: string, clear: boolean) => {
    ipcRenderer.send('SetCleared', callsign, clear)
  },
  setFlightPlanRoute: (callsign: string, route: string) => {
    ipcRenderer.send('SetFlightPlanRoute', callsign, route)
  },
  setRemarks: (callsign: string, remarks: string) => {
    ipcRenderer.send('SetRemarks', callsign, remarks)
  },
  setDepartureRunway: (callsign: string, runway: string) => {
    ipcRenderer.send('SetDepartureRunway', callsign, runway)
  },
  setSID: (callsign: string, sid: string) => {
    ipcRenderer.send('SetSID', callsign, sid)
  },
  setHeading: (callsign: string, heading: number) => {
    ipcRenderer.send(callsign, heading)
  },
}
