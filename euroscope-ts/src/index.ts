import * as net from 'net'
import { onData } from './network/DataHandler';

type ConnectFunction = () => void
type DisconnectFunction = () => void

let socket: net.Socket;

const Connect: ConnectFunction = () => {
    console.log("Trying to connect to ES!")

    // Temp values
    const PORT = 27015
    const IP = '127.0.0.1'

    socket = net.createConnection(PORT, IP)
    socket.on('data', onData)
    socket.on('close', (error) => console.log("Socket closed!"))
}

const Disconnect: DisconnectFunction = () => {
    if (socket) {
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