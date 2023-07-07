import { Message } from './Message'

export interface ControllerDataUpdated extends Message {
  $type: 'ControllerDataUpdated'
  type:
    | 'squawk'
    | 'final_altitude'
    | 'cleared_altitude'
    | 'communication_type'
    | 'ground_state'
    | 'clearence_flag'
  callsign: string
  data: boolean | string | number
}
