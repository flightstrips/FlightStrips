import * as net from 'net'
import { MessageHandlerInterface } from './MessageHandlerInterface'
import { Message } from './interfaces/Message'

export class EuroScopeSocket {
  private readonly port: number
  private readonly host: string
  private readonly handler?: MessageHandlerInterface

  constructor(handler?: MessageHandlerInterface, host?: string, port?: number) {
    if (host) {
      this.host = host
    } else {
      this.host = '127.0.0.1'
    }

    if (port) {
      this.port = port
    } else {
      this.port = 27015
    }

    this.handler = handler
  }

  public start() {
    if (this.tryReconnect) {
      this.connect()
    }
  }

  public stop() {
    this.tryReconnect = false
    if (this.socket) {
      this.clearListners()
      this.socket?.destroy()
    }
  }

  public send<T extends Message>(message: T) {
    const data = JSON.stringify(message)
    this.socket?.write(`${data}\0`)
  }

  private readonly dataBuffer: Buffer = Buffer.alloc(4096)
  private index = 0
  private socket: net.Socket | null = null
  private tryReconnect = true
  private wait = false

  private readonly delimitor: number = 0

  private onData(data: Buffer, self: this) {
    for (let i = 0; i < data.length; i++) {
      const byte = data[i]

      if (byte == this.delimitor) {
        // new message
        const bytes = this.dataBuffer.subarray(0, this.index)
        const message = new TextDecoder().decode(bytes)

        self.handler?.handleMessage(message)

        self.index = 0
        continue
      }

      self.dataBuffer[this.index++] = byte
    }
  }

  private onClose(hasError: boolean, self: this) {
    this.handler?.handleConnectionStatus(false)
    console.log(`Connection closed. Error: ${hasError}`)
    self.reconnect()
  }

  private onError(self: this) {
    this.handler?.handleConnectionStatus(false)
    self.reconnect()
  }

  private onTimeout(self: this) {
    this.handler?.handleConnectionStatus(false)
    console.log('Connection timed out!')
    self.reconnect()
  }

  private onConnected(self: this) {
    self.send({ $type: 'Initial', message: 'Client connected' })
    // TODO figure out way to avoid timeout
    setTimeout(() => this.handler?.handleConnectionStatus(true), 2500)
  }

  private clearListners() {
    if (this.socket) {
      this.socket.removeAllListeners()
    }
  }

  private connect() {
    if (!this.socket) {
      this.socket = new net.Socket()
    }

    this.wait = false
    this.socket.on('data', (data) => this.onData(data, this))
    this.socket.on('close', (hasError) => this.onClose(hasError, this))
    this.socket.on('error', () => this.onError(this))
    this.socket.on('timeout', () => this.onTimeout(this))

    this.socket.connect(this.port, this.host, () => this.onConnected(this))
  }

  private reconnect() {
    if (!this.tryReconnect || this.wait) {
      return
    }

    this.clearListners()
    this.wait = true
    setTimeout(() => this.connect(), 2500)
  }
}
