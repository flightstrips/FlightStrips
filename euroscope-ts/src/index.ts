import * as net from 'net'

type ConnectFunction = () => void

const Connect: ConnectFunction = () => {
    console.log("Trying to connect to ES!")

    // Temp values
    const PORT = 27015
    const IP = 'localhost'

    const socket = new net.Socket()
    socket.on('data', (data) => console.log(data.toString()))
    socket.connect(PORT, IP)
}

export type EuroScope = {
    connect: ConnectFunction
}

const euroScope: EuroScope = {
    connect: Connect
}

export default euroScope