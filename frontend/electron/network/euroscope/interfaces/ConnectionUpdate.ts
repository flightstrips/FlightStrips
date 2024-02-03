import { ConnectionType } from '../../../../shared/ConnectionType'
import { Message } from './Message'

export interface ConnectionUpdate extends Message {
  $type: 'ConnectionUpdate'
  connection: ConnectionType
  callsign: string
  frequency: number
}
