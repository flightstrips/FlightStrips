import * as net from 'net'
import { MessageHandlerInterface } from './MessageHandlerInterface'


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
        console.log("Starting socket connection")
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


    private readonly dataBuffer: Buffer = Buffer.alloc(4096)
    private index: number = 0
    private socket: net.Socket | null = null
    private tryReconnect: boolean = true
    private wait = false


    private readonly delimitor: number = 0

    private onData(data: Buffer, self: this) {
        for (let i = 0; i < data.length; i++) {
            const byte = data[i];

            if (byte == this.delimitor) {
                // new message
                const bytes = this.dataBuffer.subarray(0, this.index)
                const message = new TextDecoder().decode(bytes)

                self.handler?.handleMessage(message)
                console.log(message)

                self.index = 0;
                continue;
            }

            self.dataBuffer[this.index++] = byte;
        }
    }

    private onClose(hasError: boolean, self: this) {
        console.log(`Connection closed. Error: ${hasError}`)
        self.reconnect()
    }

    private onError(err: Error, self: this) {
        console.log(`Connection failed: ${err.message}`)
        self.reconnect()
    }

    private onTimeout(self: this) {
        console.log("Connection timed out!")
        self.reconnect()

    }

    private onConnected() {
        console.log("Connected")
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
        const self = this
        this.socket.on('data', data => this.onData(data, self))
        this.socket.on('close', hasError => this.onClose(hasError, self))
        this.socket.on('error', error => this.onError(error, self))
        this.socket.on('timeout', () => this.onTimeout(self))

        this.socket.connect(this.port, this.host, this.onConnected)
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