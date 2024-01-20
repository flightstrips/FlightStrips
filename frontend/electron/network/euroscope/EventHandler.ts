import { WebContents } from 'electron'
import { EuroScopeSocket } from './EuroScopeSocket'

export default class EventHandler {
  private readonly socket: EuroScopeSocket
  private readonly webContents: WebContents

  constructor(socket: EuroScopeSocket, webContents: WebContents) {
    this.socket = socket
    this.webContents = webContents
  }

  public setupHandlers() {
    this.webContents.ipc.on('SetSquawk', (_, ...args) => {
      this.socket.send({
        $type: 'SetSquawk',
        callsign: args[0],
        squawk: args[1],
      })
    })

    this.webContents.ipc.on('SetFinalAltitude', (_, ...args) => {
      this.socket.send({
        $type: 'SetFinalAltitude',
        callsign: args[0],
        altitude: args[1],
      })
    })

    this.webContents.ipc.on('SetClearedAltitude', (_, ...args) => {
      this.socket.send({
        $type: 'SetClearedAltitude',
        callsign: args[0],
        altitude: args[1],
      })
    })

    this.webContents.ipc.on('SetCommunicationType', (_, ...args) => {
      this.socket.send({
        $type: 'SetCommunicationType',
        callsign: args[0],
        communicationType: args[1],
      })
    })

    this.webContents.ipc.on('SetGroundState', (_, ...args) => {
      this.socket.send({
        $type: 'SetGroundState',
        callsign: args[0],
        state: args[1],
      })
    })

    this.webContents.ipc.on('SetCleared', (_, ...args) => {
      this.socket.send({
        $type: 'SetCleared',
        callsign: args[0],
        cleared: args[1],
      })
    })

    this.webContents.ipc.on('SetFlightPlanRoute', (_, ...args) => {
      this.socket.send({
        $type: 'SetFlightPlanRoute',
        callsign: args[0],
        route: args[1],
      })
    })

    this.webContents.ipc.on('SetRemarks', (_, ...args) => {
      console.log(`Set ${args[0]}: ${args[1]}`)
      this.socket.send({
        $type: 'SetRemarks',
        callsign: args[0],
        remarks: args[1],
      })
    })

    this.webContents.ipc.on('SetDepartureRunway', (_, ...args) => {
      this.socket.send({
        $type: 'SetDepartureRunway',
        callsign: args[0],
        runway: args[1],
      })
    })

    this.webContents.ipc.on('SetSID', (_, ...args) => {
      this.socket.send({
        $type: 'SetSID',
        callsign: args[0],
        sid: args[1],
      })
    })
    this.webContents.ipc.on('ready', () => {
      this.webContents.send(
        'EuroScopeConnectionUpdate',
        this.socket.isConnected,
      )
      this.socket.send({ $type: 'Initial' })
    })
  }
}
