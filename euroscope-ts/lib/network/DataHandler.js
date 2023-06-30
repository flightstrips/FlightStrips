let index = 0;
// for now assume messages are less than 4096 bytes.
const dataBuffer = Buffer.alloc(4096);
export function onData(data) {
    for (let i = 0; i < data.length; i++) {
        const byte = data[i];
        if (byte == 0) {
            // new message
            parseMessage(dataBuffer.subarray(0, index));
            index = 0;
            continue;
        }
        dataBuffer[index++] = byte;
    }
}
function parseMessage(bytes) {
    let json = new TextDecoder().decode(bytes);
    let obj = JSON.parse(json);
    let message = obj;
    console.log(message);
}
