export interface MessageHandlerInterface {
  handleMessage(message: string): void
  handleConnectionStatus(isConnected: boolean): void
}
