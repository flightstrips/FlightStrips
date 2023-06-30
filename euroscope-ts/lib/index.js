import * as net from 'net';
import { onData } from './network/DataHandler';
import { setTimeout } from 'timers';
let socket;
let shouldReconnect = true;
let waitConnect = false;
function internalConnect() {
    console.info("Trying to connect to ES!");
    waitConnect = false;
    // Temp values
    const PORT = 27015;
    const IP = '127.0.0.1';
    if (!socket) {
        socket = net.createConnection(PORT, IP, onConnected);
    }
    else {
        socket.connect(PORT, IP, onConnected);
    }
    socket.on('data', onData);
    socket.on('close', onClose);
    socket.on('error', onError);
    socket.on('timeout', () => console.info('Timed out'));
}
function onConnected() {
    console.info("Connected");
}
function onClose(hasError) {
    console.info("Socket closed.");
    socket.removeAllListeners();
    tryReconnect();
}
function onError(err) {
    console.info(`Failed to connect ${err.message}`);
    socket.removeAllListeners();
    tryReconnect();
}
function tryReconnect() {
    if (shouldReconnect && !waitConnect) {
        waitConnect = true;
        setTimeout(() => internalConnect(), 2500);
    }
}
const Connect = () => {
    internalConnect();
};
const Disconnect = () => {
    shouldReconnect = false;
    if (socket) {
        socket.removeAllListeners();
        socket.destroy();
    }
};
const euroScope = {
    connect: Connect,
    disconnect: Disconnect
};
export default euroScope;
