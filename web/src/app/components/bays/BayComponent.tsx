import { CurrentUTC } from "@/app/helpers/time";
import { FlightStrip } from "../strip/FlightStrip";
import { Message } from "../Message";

export default function BayComponent() {
    return (<>
        <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2 aspect-video">
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        OTHERS
                    </span>
                    <span className="flex gap-2">
                        <button className="bg-[#646464] text-white font-bold text-lg px-4 border-2 border-white active:bg-[#424242]">
                            NEW
                        </button>
                        <button className="bg-[#646464] text-white font-bold text-lg px-4 border-2 border-white active:bg-[#424242]">
                            PLANNED
                        </button>
                    </span>
                </div>
                <div className="h-[calc(100%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                    <FlightStrip callsing="DAT3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="BAW1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'EGLL'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="DLH5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'EDDF'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="AFR4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'LFPG'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="KLM8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="RYR2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'EIDW'} stand={'A32'} tsat={'1500'}/>
                    <FlightStrip callsing="EZY3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'EGKK'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="WZZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'LHBP'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="UAL5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'KORD'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="AAL4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'KJFK'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="QFA8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'YSSY'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="ANA2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'RJTT'} stand={'A32'} tsat={'1500'}/>
                    <FlightStrip callsing="JAL3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'RJAA'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="SIA1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'WSSS'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="THA5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'VTBS'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="MAS4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'WMKK'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="CXA8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'VHHH'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="EVA2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'RCTP'} stand={'A32'} tsat={'1500'}/>
                    <FlightStrip callsing="KAL3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'RKSI'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="ETD1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'OMAA'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="QTR5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'OTHH'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="GIA4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'WIII'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="VIR8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'EGLL'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="BAW2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'EGLL'} stand={'A32'} tsat={'1500'}/>
                    <FlightStrip callsing="DLH3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'EDDF'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="AFR1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'LFPG'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="KLM5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'EHAM'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="RYR4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'EIDW'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="EZY8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'EGKK'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="WZZ2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'LHBP'} stand={'A32'} tsat={'1500'}/>
                </div>
            </div>
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        SAS
                    </span>
                </div>
                <div className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                <FlightStrip callsing="NSZ3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="SAS1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="SAS5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="SAS4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="SAS8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="SAS2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'}/>
                    <FlightStrip callsing="SAS3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="SAS1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="SAS5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="SAS4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="SAS8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="SAS2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'}/>
                </div>
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        NORWEGIAN
                    </span>
                </div>
                <div className="h-[calc(50%-2.4rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                    <FlightStrip callsing="NSZ3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="NAX5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="NOZ4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="WIR8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="NOZ2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'}/>
                    <FlightStrip callsing="NSZ3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'}/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'}/>
                    <FlightStrip callsing="NAX5678" clearances standchanged taxiway="B" holdingpoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'}/>
                    <FlightStrip callsing="NOZ4321" clearances standchanged taxiway="C" holdingpoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'}/>
                    <FlightStrip callsing="WIR8765" clearances standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'}/>
                    <FlightStrip callsing="NOZ2345" clearances standchanged taxiway="E" holdingpoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'}/>
                </div>
            </div>
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        CLEARED
                    </span>
                </div>
                <div className="h-1/2 w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
                <div className="bg-[#285a5c] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        MESSAGES
                    </span>
                </div>
                <div className="h-[calc(50%-5rem)] w-full bg-[#555355]">
                    <Message>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Nam tincidunt vitae enim eget porttitor. Suspendisse ultrices ullamcorper tortor, vitae condimentum lacus convallis at. </Message>
                    <Message><b>FLIGHTSTRIPS</b> has deteced that EKCH_DEL has logged off. You are in change of Delivery!</Message>
                    <Message>VFR Request LOW PASS rwy 12</Message>
                </div>
            </div>
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
                    <span className="text-[#393939] font-bold text-lg">
                        PUSHBACK
                    </span>
                </div>
                <div className="h-2/5 w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
                <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
                    <span className="text-[#393939] font-bold text-lg">
                        TWY DEP
                    </span>
                </div>
                <div className="h-[calc(60%-5rem)] w-full bg-[#555355]">
                    Lorem ipsum dolor sit amet, consectetur adipisicing elit. Fugit veniam minima laudantium distinctio nisi unde eveniet quibusdam similique, atque laboriosam asperiores ducimus nam maxime odit at sapiente. Similique, eveniet neque.
                </div>
            </div>
        </div>
        <div className="h-16 w-screen bg-[#3b3b3b] flex justify-between">
            <div className="h-full w-full flex">
                <div className="bg-[#1bff16] text-black w-32 flex justify-center items-center m-2 font-bold">
                    CLR DEL
                </div>
                <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
                    <h1>
                        DEP
                    </h1>
                    <span className="bg-white text-black w-16 p-2">
                        22R
                    </span>
                </div>
                <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
                    <h1>
                        ARR
                    </h1>
                    <span className="bg-white text-black w-16 p-2">
                        22L
                    </span>
                </div>
                <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
                    <h1>
                        QNH
                    </h1>
                    <span className="bg-[#212121]  w-18 p-2">
                        1015
                    </span>
                </div>
            </div>
            <div className="flex items-center justify-center gap-1">
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    TRF
                </button>
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    MRK
                </button>
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    REQ
                </button>
                <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                    X
                </button>
                <div className="w-32 bg-[#e4e4e4] text-black flex items-center justify-center p-3">
                    <CurrentUTC />
                </div>
            </div>
        </div>
    </>) 
}