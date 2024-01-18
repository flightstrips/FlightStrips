export class Controller {
  callsign: string
  position = 'XX'
  frequency = '199.998'

  constructor(callsign: string) {
    this.callsign = callsign
  }
}
