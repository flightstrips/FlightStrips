import { Message } from './Message'

export interface ControllerUpdate extends Message {
  $type: 'ControllerUpdate'
  callsign: string
  frequency: number
  position: string
}
