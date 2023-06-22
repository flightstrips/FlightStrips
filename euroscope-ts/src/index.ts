import * as net from 'net'
import { FlightDataUpdatedMessage } from './interfaces/FlightDataUpdatedMessage';
import { Message } from './interfaces/Message';

type ConnectFunction = () => void

const Connect: ConnectFunction = () => {
    console.log("Trying to connect to ES!")

    // Temp values
    const PORT = 27015
    const IP = '127.0.0.1'

    const socket = net.createConnection(PORT, IP)
    // TODO better handling, this can miss messages and does not handle partial messages
    socket.on('data', (data) => {
        const index = data.indexOf(0x00)
        if (index != -1) {
            let slice = data.buffer.slice(0, index)
            let json = new TextDecoder().decode(slice)
            let obj = JSON.parse(json)
            let message = obj as FlightDataUpdatedMessage | Message

            if (message.$type == 'FlightPlanUpdated') {
                console.log(message)
            }
        }
    })
}

export type EuroScope = {
    connect: ConnectFunction
}

const euroScope: EuroScope = {
    connect: Connect
}

export default euroScope