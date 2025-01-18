import { CurrentUTC } from "@/app/helpers/time";
import { FlightStrip } from "../strip/FlightStrip";
import { Message } from "../Message";
import CommandBar from "../commandbar/CommandBar";

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
                    <FlightStrip callsing="DAT3676" standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="BAW1234" standchanged taxiway="A" holdingpoint="B1" destination={'EGLL'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="DLH5678" standchanged taxiway="B" holdingpoint="C2" destination={'EDDF'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="AFR4321" standchanged taxiway="C" holdingpoint="D3" destination={'LFPG'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="KLM8765" standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="RYR2345" standchanged taxiway="E" holdingpoint="F5" destination={'EIDW'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsing="EZY3676" standchanged taxiway="D" holdingpoint="E1" destination={'EGKK'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="WZZ1234" standchanged taxiway="A" holdingpoint="B1" destination={'LHBP'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="UAL5678" standchanged taxiway="B" holdingpoint="C2" destination={'KORD'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="AAL4321" standchanged taxiway="C" holdingpoint="D3" destination={'KJFK'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="QFA8765" standchanged taxiway="D" holdingpoint="E4" destination={'YSSY'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="ANA2345" standchanged taxiway="E" holdingpoint="F5" destination={'RJTT'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsing="JAL3676" standchanged taxiway="D" holdingpoint="E1" destination={'RJAA'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="SIA1234" standchanged taxiway="A" holdingpoint="B1" destination={'WSSS'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="THA5678" standchanged taxiway="B" holdingpoint="C2" destination={'VTBS'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="MAS4321" standchanged taxiway="C" holdingpoint="D3" destination={'WMKK'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="CXA8765" standchanged taxiway="D" holdingpoint="E4" destination={'VHHH'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="EVA2345" standchanged taxiway="E" holdingpoint="F5" destination={'RCTP'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsing="KAL3676" standchanged taxiway="D" holdingpoint="E1" destination={'RKSI'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="ETD1234" standchanged taxiway="A" holdingpoint="B1" destination={'OMAA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="QTR5678" standchanged taxiway="B" holdingpoint="C2" destination={'OTHH'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="GIA4321" standchanged taxiway="C" holdingpoint="D3" destination={'WIII'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="VIR8765" standchanged taxiway="D" holdingpoint="E4" destination={'EGLL'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="BAW2345" standchanged taxiway="E" holdingpoint="F5" destination={'EGLL'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsing="DLH3676" standchanged taxiway="D" holdingpoint="E1" destination={'EDDF'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="AFR1234" standchanged taxiway="A" holdingpoint="B1" destination={'LFPG'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="KLM5678" standchanged taxiway="B" holdingpoint="C2" destination={'EHAM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="RYR4321" standchanged taxiway="C" holdingpoint="D3" destination={'EIDW'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="EZY8765" standchanged taxiway="D" holdingpoint="E4" destination={'EGKK'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="WZZ2345" standchanged taxiway="E" holdingpoint="F5" destination={'LHBP'} stand={'A32'} tsat={'1500'} status="CLR"/>
                </div>
            </div>
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        SAS
                    </span>
                </div>
                <div className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                <FlightStrip callsing="OYYSB" standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="SAS1234" standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="SAS5678" standchanged taxiway="B" holdingpoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="SAS4321" standchanged taxiway="C" holdingpoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="SAS8765" standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="SAS2345" standchanged taxiway="E" holdingpoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsing="SAS3676" standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="SAS1234" standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="SAS5678" standchanged taxiway="B" holdingpoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="SAS4321" standchanged taxiway="C" holdingpoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="SAS8765" standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="SAS2345" standchanged taxiway="E" holdingpoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'} status="CLR"/>
                </div>
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        NORWEGIAN
                    </span>
                </div>
                <div className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                    <FlightStrip callsing="NSZ3676" standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="NSZ1234" standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="NAX5678" standchanged taxiway="B" holdingpoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="NOZ4321" standchanged taxiway="C" holdingpoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="WIF8765" standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="NOZ2345" standchanged taxiway="E" holdingpoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'} status="CLR"/>
                    <FlightStrip callsing="NSZ3676" standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLR"/>
                    <FlightStrip callsing="NSZ1234" standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLR"/>
                    <FlightStrip callsing="NAX5678" standchanged taxiway="B" holdingpoint="C2" destination={'ENGM'} stand={'B9'} tsat={'1415'} status="CLR"/>
                    <FlightStrip callsing="NOZ4321" standchanged taxiway="C" holdingpoint="D3" destination={'EGLL'} stand={'B4'} tsat={'1430'} status="CLR"/>
                    <FlightStrip callsing="WIF8765" standchanged taxiway="D" holdingpoint="E4" destination={'EHAM'} stand={'A20'} tsat={'1445'} status="CLR"/>
                    <FlightStrip callsing="NOZ2345" standchanged taxiway="E" holdingpoint="F5" destination={'LFPG'} stand={'A32'} tsat={'1500'} status="CLR"/>
                </div>
            </div>
            <div className="w-1/4 h-full bg-[#555355]">
                <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        CLEARED
                    </span>
                </div>
                <div className="h-1/2 w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                <FlightStrip callsing="NSZ3676" clearances standchanged taxiway="D" holdingpoint="E1" destination={'EKYT'} stand={'D3'} tsat={'1312'} status="CLROK"/>
                <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="CLROK"/>
                </div>
                <div className="bg-[#285a5c] h-10 flex items-center px-2 justify-between">
                    <span className="text-white font-bold text-lg">
                        MESSAGES
                    </span>
                </div>
                <div className="h-[calc(50%-6rem)] w-full bg-[#555355]">
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
                <div className="h-2/5 w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                </div>
                <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
                    <span className="text-[#393939] font-bold text-lg">
                        TWY DEP
                    </span>
                </div>
                <div className="h-[calc(60%-5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-[#285a5c]">
                <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                    <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF"/>
                </div>
            </div>
        </div>
        <CommandBar />
    </>) 
}