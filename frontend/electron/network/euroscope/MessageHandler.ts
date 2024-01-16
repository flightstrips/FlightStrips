import { CommunicationType } from '../../../shared/CommunicationType'
import { MessageHandlerInterface } from './MessageHandlerInterface'
import { ControllerDataUpdated } from './interfaces/ControllerDataUpdated'
import { FlightDataUpdatedMessage } from './interfaces/FlightDataUpdatedMessage'
import { FlightPlanDisconnected } from './interfaces/FlightPlanDisconnected'
import { IpcInterface } from './interfaces/IpcInterface'
import { Message } from './interfaces/Message'
import { SquawkUpdate } from './interfaces/SquawkUpdate'
import { ActiveRunwaysMessage } from './interfaces/ActiveRunwaysMessage'
import { GroundState } from '../../../shared/GroundState'

export class MessageHandler implements MessageHandlerInterface {
  private readonly ipc: IpcInterface

  constructor(ipc: IpcInterface) {
    this.ipc = ipc
  }

  handleMessage(message: string): void {
    const event = JSON.parse(message) as Message

    switch (event.$type) {
      case 'ControllerUpdate':
        console.log(`Controller update: ${message}`)
        break
      case 'FlightPlanUpdated':
        this.ipc.sendFlightPlanUpdate(event as FlightDataUpdatedMessage)
        break
      case 'ControllerDataUpdated': {
        const controllerUpdate = event as ControllerDataUpdated
        switch (controllerUpdate.type) {
          case 'cleared_altitude':
            this.ipc.sendSetClearedAltitude(
              controllerUpdate.callsign,
              controllerUpdate.data as number,
            )
            break
          case 'clearence_flag':
            this.ipc.sendSetCleared(
              controllerUpdate.callsign,
              controllerUpdate.data as boolean,
            )
            break
          case 'communication_type':
            this.ipc.sendSetCommunicationType(
              controllerUpdate.callsign,
              controllerUpdate.data as CommunicationType,
            )
            break
          case 'final_altitude':
            this.ipc.sendSetFinalAltitude(
              controllerUpdate.callsign,
              controllerUpdate.data as number,
            )
            break
          case 'ground_state':
            this.ipc.sendSetGroundState(
              controllerUpdate.callsign,
              controllerUpdate.data as GroundState,
            )
            break
          case 'squawk':
            this.ipc.sendSetSquawk(
              controllerUpdate.callsign,
              controllerUpdate.data as string,
            )
            break
          default:
            console.error(
              `Unknown controller data update type '${controllerUpdate.type}'.`,
            )
        }
        break
      }
      case 'FlightPlanDisconnected':
        this.ipc.sendFlightPlanDisconnect(
          (event as FlightPlanDisconnected).callsign,
        )
        break
      case 'SquawkUpdate': {
        const squawkUpdate = event as SquawkUpdate
        this.ipc.sendSquawkUpdate(squawkUpdate.callsign, squawkUpdate.squawk)
        break
      }
      case 'ActiveRunways': {
        const runways = event as ActiveRunwaysMessage
        this.ipc.sendActiveRunways(runways.runways)
        break
      }
      default:
        console.error(
          `Unknown message type '${event.$type}'. Raw JSON: ${message}`,
        )
    }
  }
}
