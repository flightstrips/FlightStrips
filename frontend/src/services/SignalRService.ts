import * as signalR from '@microsoft/signalr'
import { HttpTransportType } from '@microsoft/signalr'

export class SignalRService {
  private connection: signalR.HubConnection

  constructor() {
    this.connection = new signalR.HubConnectionBuilder()
      .withUrl('http://localhost:5233/hubs/events', {
        skipNegotiation: true,
        withCredentials: false,
        transport: HttpTransportType.WebSockets,
      })
      .configureLogging(signalR.LogLevel.Information)
      .withAutomaticReconnect()
      .build()

    this.connection
      .start()
      .catch((err) => console.log(`Failed to connect to backend ${err}`))
  }

  public tryReconnect() {
    if (this.connection.state !== signalR.HubConnectionState.Disconnected) {
      return
    }

    this.connection
      .start()
      .catch((err) => console.log(`Failed to connect to backend ${err}`))
  }

  public getState(): 'Connected' | 'Disconnected' | 'Connecting' {
    switch (this.connection.state) {
      case signalR.HubConnectionState.Connected:
        return 'Connected'
      case signalR.HubConnectionState.Disconnected:
      case signalR.HubConnectionState.Disconnecting:
        return 'Disconnected'
      case signalR.HubConnectionState.Connecting:
      case signalR.HubConnectionState.Reconnecting:
        return 'Connecting'
    }
  }

  public subscribeAirport(session: string): Promise<void> {
    return this.connection.invoke('subscribeAirport', {
      Airport: 'EKCH',
      Session: session,
    })
  }

  public subscribe(
    session: string,
    callsign: string,
    frequency: string,
  ): Promise<void> {
    return this.connection.invoke('subscribe', {
      Airport: 'EKCH',
      Session: session,
      Frequency: frequency,
      callsign: callsign,
    })
  }

  public unsubscribe(session: string, frequency: string): Promise<void> {
    return this.connection.invoke('unsubscribe', {
      Airport: 'EKCH',
      Session: session,
      Frequency: frequency,
    })
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  public on(eventName: string, callback: (...args: any[]) => void) {
    this.connection.on(eventName, callback)
  }

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  public off(eventName: string, callback: (...args: any[]) => void) {
    this.connection.off(eventName, callback)
  }
}

export const signalRService = new SignalRService()
