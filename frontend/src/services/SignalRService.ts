import * as signalR from '@microsoft/signalr'

export class SignalRService {
  private connection: signalR.HubConnection

  constructor() {
    this.connection = new signalR.HubConnectionBuilder()
      .withUrl('http://localhost:5233/hubs/events')
      .configureLogging(signalR.LogLevel.Information)
      .build()

    this.connection
      .start()
      .catch((err) => console.error('SignalR Connection Error: ', err))
  }

  public on(eventName: string, callback: (...args: any[]) => void) {
    this.connection.on(eventName, callback)
  }

  public off(eventName: string, callback: (...args: any[]) => void) {
    this.connection.off(eventName, callback)
  }
}

export const signalRService = new SignalRService()
