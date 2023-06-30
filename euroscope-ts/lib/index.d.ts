type ConnectFunction = () => void;
type DisconnectFunction = () => void;
export type EuroScope = {
    connect: ConnectFunction;
    disconnect: DisconnectFunction;
};
declare const euroScope: EuroScope;
export default euroScope;
