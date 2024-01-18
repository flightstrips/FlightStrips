import { Message } from './Message'

export interface ControllerDisconect extends Message {
  $type: 'ControllerDisconnect'
  callsign: string
  frequency: number
}
