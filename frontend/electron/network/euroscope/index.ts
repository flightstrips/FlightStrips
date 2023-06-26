import * as net from 'net'
import { onData } from './parser/DataHandler';
import { setTimeout } from 'timers';
import { BrowserWindow } from 'electron';

// VERY HACK
type ConnectFunction = (window: BrowserWindow) => void
type DisconnectFunction = () => void

let socket: net.Socket;
let shouldReconnect = true
let waitConnect = false;

let browserWindow: BrowserWindow | null


function internalConnect() {
    console.info("Trying to connect to ES!")
    waitConnect = false;

    // Temp values
    const PORT = 27015
    const IP = '127.0.0.1'

    if (!socket) {
        socket = net.createConnection(PORT, IP, onConnected)
    } else {
        socket.connect(PORT, IP, onConnected)
    }
    socket.on('data', (data) => onData(data, browserWindow as BrowserWindow))
    socket.on('close', onClose)
    socket.on('error', onError)
    socket.on('timeout', () => console.info('Timed out'))
}

function onConnected() {
    console.info("Connected")

}

function onClose(hasError: boolean) {
    console.info("Socket closed.")
    socket.removeAllListeners()
    tryReconnect()
}

function onError(err: Error) {
    console.info(`Failed to connect ${err.message}`)
    socket.removeAllListeners()
    tryReconnect()
}

function tryReconnect() {
    if (shouldReconnect && !waitConnect) {
        waitConnect = true;
        setTimeout(() => internalConnect(), 2500)
    }
}

const Connect: ConnectFunction = (window: BrowserWindow) => {
    browserWindow = window
    internalConnect()
}

const Disconnect: DisconnectFunction = () => {
    shouldReconnect = false
    browserWindow = null
    if (socket) {
        socket.removeAllListeners()
        socket.destroy()
    }
}

export type EuroScope = {
    connect: ConnectFunction
    disconnect: DisconnectFunction
}

const euroScope: EuroScope = {
    connect: Connect,
    disconnect: Disconnect

}

export default euroScope